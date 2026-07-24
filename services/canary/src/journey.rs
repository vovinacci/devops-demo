use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use reqwest::Client;
use reqwest::header::HeaderMap;
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::metrics::Metrics;
use crate::otel::{current_trace_id, inject_traceparent};

/// Prefix tagging every item the canary creates (ADR-0007 D9): the
/// synthetic-traffic tag that lets dashboards and report queries filter
/// canary data out of business metrics while it still flows through the
/// real pipeline.
pub const ITEM_PREFIX: &str = "canary-";

#[derive(Debug, Serialize)]
struct ItemCreate<'a> {
    name: &'a str,
}

#[derive(Debug, Deserialize)]
struct Item {
    id: i64,
    name: String,
}

#[derive(Debug, PartialEq, Eq)]
pub enum Outcome {
    Success,
    Failure,
}

/// Config for the v2 pipeline-lag step (RFC-0001 D9, ADR-0008 D10). Kept
/// separate from `config::Config` so `journey::run` stays testable
/// without constructing a whole env-derived `Config`.
pub struct PipelineConfig<'a> {
    pub analytics_url: &'a str,
    pub timeout: Duration,
    pub poll_interval: Duration,
}

/// Config for the v3 report step (RFC-0001 D9, ADR-0008 D10). Kept
/// separate from `config::Config`, exactly like `PipelineConfig`, so
/// `journey::run` stays testable without an env-derived `Config`.
pub struct ReportConfig<'a> {
    pub reports_url: &'a str,
    pub timeout: Duration,
    pub poll_interval: Duration,
}

/// Three-valued outcome of the pipeline-lag step. None of the three
/// changes the journey's own success/failure verdict (Hard rule 9,
/// ADR-0008 D10): a down or slow analytics is a pipeline signal, not a
/// canary journey failure.
#[derive(Debug, PartialEq, Eq)]
pub enum PipelineOutcome {
    /// Item became visible in analytics within `timeout`.
    Ok,
    /// Analytics was reachable throughout but never showed the item --
    /// the pipeline-lag signal this step exists to catch (the D3 gRPC
    /// event-stream durability gap, ADR-0002).
    Timeout,
    /// Analytics was unreachable (connection refused / DNS failure) on
    /// the first poll -- read as "the `analytics` profile is not up"
    /// (ADR-0008 D10), not a pipeline failure.
    Skipped,
}

impl PipelineOutcome {
    fn as_label(&self) -> &'static str {
        match self {
            PipelineOutcome::Ok => "ok",
            PipelineOutcome::Timeout => "timeout",
            PipelineOutcome::Skipped => "skipped",
        }
    }
}

/// Four-valued outcome of the v3 report step. Like `PipelineOutcome`,
/// none of the four changes the journey's own success/failure verdict
/// (Hard rule 9, ADR-0008 D10): reports being down, slow, or failing a
/// render is a reports signal, not a canary journey failure.
#[derive(Debug, PartialEq, Eq)]
pub enum ReportOutcome {
    /// A submitted job reached `SUCCEEDED` within `timeout`; generation
    /// latency (submit -> `SUCCEEDED`) is recorded in
    /// `report_generation_seconds`.
    Ok,
    /// Reports was reachable but the job never reached a terminal state
    /// within `timeout` -- the report-lag signal this step exists to catch.
    Timeout,
    /// The job reached the terminal `FAILED` state -- reports was up and
    /// answered, the render itself failed.
    Failed,
    /// The submit POST itself could not connect (connection refused / DNS
    /// failure) -- read as "the `reports` profile is not up" (ADR-0008
    /// D10), not a journey failure. Once the POST is accepted, reports is
    /// proven reachable, so a later poll error is transient (-> `Timeout`),
    /// never `Skipped`.
    Skipped,
}

impl ReportOutcome {
    fn as_label(&self) -> &'static str {
        match self {
            ReportOutcome::Ok => "ok",
            ReportOutcome::Timeout => "timeout",
            ReportOutcome::Failed => "failed",
            ReportOutcome::Skipped => "skipped",
        }
    }
}

/// Executes one canary journey: create -> verify -> pipeline-lag ->
/// report -> clean up (RFC-0001 D9 v3). The pipeline-lag step arrived
/// with analytics (Phase 3); the report step arrived with reports
/// (Phase 6). The item is deleted even when verify, the pipeline-lag
/// step, or the report step fails or is skipped -- best-effort cleanup,
/// never left for the next run.
///
/// `#[instrument]` opens the root span for the journey: this is what
/// gives the OpenTelemetry layer something to assign a real trace ID to
/// (D11) -- without an active span, `current_trace_id` below would read
/// back an all-zero ID.
#[tracing::instrument(name = "canary_journey", skip_all)]
pub async fn run(
    client: &Client,
    backend_url: &str,
    timeout: Duration,
    metrics: &Metrics,
    pipeline: &PipelineConfig<'_>,
    report: &ReportConfig<'_>,
) -> Outcome {
    let trace_id = current_trace_id();
    let name = format!("{ITEM_PREFIX}{}", Uuid::new_v4());
    tracing::info!(trace_id, item_name = %name, "canary journey started");

    let created = time_step(
        metrics,
        "create",
        create_item(client, backend_url, &name, timeout),
    )
    .await;

    let Some(item) = created else {
        tracing::warn!(
            trace_id,
            "canary journey failed: create step did not succeed"
        );
        metrics.journey_total.with_label_values(&["failure"]).inc();
        return Outcome::Failure;
    };
    // Pipeline lag is measured from here -- the create call's completion
    // -- not from when the pipeline-lag step itself starts polling, so
    // the recorded lag reflects the full create -> ... -> visible path
    // (RFC-0001 D9 v2), including the verify step's own time in between.
    let created_at = Instant::now();

    let verified = time_step(
        metrics,
        "verify",
        verify_item(client, backend_url, item.id, &name, timeout),
    )
    .await;

    // Runs regardless of the verify outcome -- it measures the
    // create -> stream -> analytics-ingest -> read-API path (D3 gRPC
    // durability gap, ADR-0002), independent of backend read-your-write
    // semantics. Placed before delete: the item must still exist on the
    // backend side for this to test the real ingestion path, though
    // analytics visibility itself is keyed on tombstoned state, not
    // backend existence (a deleted-but-once-seen item still reads 200).
    let pipeline_outcome = time_step(
        metrics,
        "pipeline",
        pipeline_lag_step(client, pipeline, item.id, created_at, metrics),
    )
    .await;
    metrics
        .pipeline_check_total
        .with_label_values(&[pipeline_outcome.as_label()])
        .inc();

    // Best-effort v3 report step: submits a report job to reports and
    // waits for it to reach a terminal state (RFC-0001 D9 v3). Runs
    // regardless of the verify/pipeline outcome and, like the pipeline
    // step, its own result never changes the journey verdict (Hard rule
    // 9, ADR-0008 D10). Placed after pipeline and before delete so the
    // item it summarizes still exists on the backend side.
    let report_outcome = time_step(metrics, "report", report_step(client, report, metrics)).await;
    metrics
        .report_check_total
        .with_label_values(&[report_outcome.as_label()])
        .inc();

    // Best-effort: attempted regardless of the verify/pipeline/report
    // outcome, and its own result never changes the journey's
    // success/failure verdict.
    let _ = time_step(
        metrics,
        "delete",
        delete_item(client, backend_url, item.id, timeout),
    )
    .await;

    if verified == Some(true) {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs_f64();
        metrics.last_success_timestamp_seconds.set(now);
        metrics.journey_total.with_label_values(&["success"]).inc();
        tracing::info!(trace_id, "canary journey succeeded");
        Outcome::Success
    } else {
        tracing::warn!(
            trace_id,
            "canary journey failed: item not visible in verify step"
        );
        metrics.journey_total.with_label_values(&["failure"]).inc();
        Outcome::Failure
    }
}

/// Times a step and records it to the per-step histogram regardless of
/// outcome -- failed steps still need a latency data point so the
/// runbook query ("which step is slow before it fails") works.
async fn time_step<T, F>(metrics: &Metrics, step: &str, fut: F) -> T
where
    F: std::future::Future<Output = T>,
{
    let start = Instant::now();
    let result = fut.await;
    metrics
        .step_duration_seconds
        .with_label_values(&[step])
        .observe(start.elapsed().as_secs_f64());
    result
}

fn traced_headers() -> HeaderMap {
    let mut headers = HeaderMap::new();
    inject_traceparent(&mut headers);
    headers
}

async fn create_item(
    client: &Client,
    backend_url: &str,
    name: &str,
    timeout: Duration,
) -> Option<Item> {
    let resp = client
        .post(format!("{backend_url}/items"))
        .headers(traced_headers())
        .json(&ItemCreate { name })
        .timeout(timeout)
        .send()
        .await
        .ok()?;
    if !resp.status().is_success() {
        return None;
    }
    resp.json::<Item>().await.ok()
}

/// `None` = the verify request itself did not succeed (network error,
/// non-2xx); `Some(false)` = the request succeeded but the item is not
/// (yet) visible; `Some(true)` = verified. Both `None` and `Some(false)`
/// are journey failures -- kept distinct because only the latter is a
/// genuine correctness gap rather than a reachability problem.
async fn verify_item(
    client: &Client,
    backend_url: &str,
    id: i64,
    name: &str,
    timeout: Duration,
) -> Option<bool> {
    let resp = client
        .get(format!("{backend_url}/items"))
        .headers(traced_headers())
        .timeout(timeout)
        .send()
        .await
        .ok()?;
    if !resp.status().is_success() {
        return None;
    }
    let items: Vec<Item> = resp.json().await.ok()?;
    Some(items.iter().any(|i| i.id == id && i.name == name))
}

async fn delete_item(client: &Client, backend_url: &str, id: i64, timeout: Duration) -> Option<()> {
    let resp = client
        .delete(format!("{backend_url}/items/{id}"))
        .headers(traced_headers())
        .timeout(timeout)
        .send()
        .await
        .ok()?;
    resp.status().is_success().then_some(())
}

/// Polls analytics's `GET /api/v1/items/{id}` until it returns any 2xx
/// (analytics's own contract: 200 covers both a live item and a
/// tombstoned one, distinct from 404 "never seen" -- see
/// `services/analytics/internal/httpserver/server.go` `handleGetItem`;
/// the item here was just created, so any 200 means the create -> stream
/// -> ingest -> read-API path completed). Response bodies are never
/// parsed -- status alone answers "is it known yet".
///
/// Only the *first* poll's connect error is read as "analytics is not up"
/// (`PipelineOutcome::Skipped`, ADR-0008 D10): a connect error on a later
/// attempt, after analytics was reachable moments ago, is far more likely
/// a transient blip than the whole profile disappearing mid-step, so it
/// is treated the same as any other non-2xx/non-connect response --
/// kept polling until `pipeline.timeout`, which then reports
/// `PipelineOutcome::Timeout` rather than `Skipped`.
async fn pipeline_lag_step(
    client: &Client,
    pipeline: &PipelineConfig<'_>,
    item_id: i64,
    created_at: Instant,
    metrics: &Metrics,
) -> PipelineOutcome {
    let url = format!("{}/api/v1/items/{item_id}", pipeline.analytics_url);
    let deadline = Instant::now() + pipeline.timeout;
    let mut first_attempt = true;

    loop {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            return PipelineOutcome::Timeout;
        }

        match client
            .get(&url)
            .headers(traced_headers())
            .timeout(remaining)
            .send()
            .await
        {
            Ok(resp) if resp.status().is_success() => {
                metrics
                    .pipeline_lag_seconds
                    .observe(created_at.elapsed().as_secs_f64());
                return PipelineOutcome::Ok;
            }
            // 404 (never seen) or any other non-2xx: analytics is
            // reachable, the item just is not visible yet -- keep polling.
            Ok(_not_yet_visible) => {}
            Err(err) if first_attempt && err.is_connect() => {
                return PipelineOutcome::Skipped;
            }
            Err(_transient) => {}
        }

        first_attempt = false;
        tokio::time::sleep(pipeline.poll_interval.min(remaining)).await;
    }
}

#[derive(Debug, Serialize)]
struct ReportRequest {
    // The reports API's job type field is JSON `type`, a Rust keyword.
    #[serde(rename = "type")]
    kind: &'static str,
    format: &'static str,
}

#[derive(Debug, Deserialize)]
struct ReportStatus {
    status: String,
}

/// Submits a report job to reports and waits for it to reach a terminal
/// state, mirroring `pipeline_lag_step`. `format: "csv"` is chosen
/// deliberately -- the lightest artifact, needing no Apache POI / OpenPDF
/// render -- so the canary costs less than the reports workload it
/// exercises (RFC-0001 D9: a monitor must cost less than what it
/// monitors). The generated file is never downloaded: a `SUCCEEDED`
/// status already proves the async job API (submit -> render -> terminal)
/// worked end to end, so a download would move bytes for no extra signal.
///
/// Only the submit POST's own connect error is read as "reports is not up"
/// (`ReportOutcome::Skipped`, ADR-0008 D10). Once the POST is accepted
/// (202), reports is proven reachable, so any later poll error -- connect
/// or otherwise -- is a transient blip and polled through to `Timeout`,
/// never re-labelled `Skipped`.
async fn report_step(
    client: &Client,
    report: &ReportConfig<'_>,
    metrics: &Metrics,
) -> ReportOutcome {
    let deadline = Instant::now() + report.timeout;
    let submitted_at = Instant::now();

    // Submit the job. A connect error here means the reports profile is
    // not up -> Skipped; any other submit failure leaves nothing to poll.
    let submit = client
        .post(format!("{}/reports", report.reports_url))
        .headers(traced_headers())
        .json(&ReportRequest {
            kind: "items-summary",
            format: "csv",
        })
        .timeout(report.timeout)
        .send()
        .await;
    let resp = match submit {
        Ok(resp) => resp,
        Err(err) if err.is_connect() => return ReportOutcome::Skipped,
        Err(_transient) => return ReportOutcome::Timeout,
    };

    // Expect 202 Accepted with a Location header ("/reports/{id}", the
    // reports API's status resource). Anything else means the job was not
    // accepted -- reachable, but never terminal -> Timeout.
    if resp.status() != reqwest::StatusCode::ACCEPTED {
        return ReportOutcome::Timeout;
    }
    let Some(location) = resp
        .headers()
        .get(reqwest::header::LOCATION)
        .and_then(|v| v.to_str().ok())
        .map(str::to_owned)
    else {
        return ReportOutcome::Timeout;
    };
    let status_url = format!("{}{location}", report.reports_url);

    // Poll the status resource until the job reaches a terminal state or
    // the deadline elapses. Non-2xx / non-terminal / later transient
    // errors keep the loop going; only the deadline yields Timeout.
    loop {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            return ReportOutcome::Timeout;
        }

        match client
            .get(&status_url)
            .headers(traced_headers())
            .timeout(remaining)
            .send()
            .await
        {
            Ok(resp) if resp.status().is_success() => match resp.json::<ReportStatus>().await {
                Ok(status) if status.status == "SUCCEEDED" => {
                    metrics
                        .report_generation_seconds
                        .observe(submitted_at.elapsed().as_secs_f64());
                    return ReportOutcome::Ok;
                }
                Ok(status) if status.status == "FAILED" => return ReportOutcome::Failed,
                // PENDING / RUNNING, or a body that did not parse -- not
                // terminal yet, keep polling.
                _ => {}
            },
            // Non-2xx (e.g. a 404 before the row is queryable): reachable,
            // not terminal yet -- keep polling.
            Ok(_not_terminal) => {}
            // The POST already proved reports reachable; any poll error is a
            // transient blip, polled through to Timeout, never Skipped.
            Err(_transient) => {}
        }

        tokio::time::sleep(report.poll_interval.min(remaining)).await;
    }
}
