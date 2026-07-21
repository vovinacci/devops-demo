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

/// Executes one canary journey: create -> verify -> clean up (RFC-0001
/// D9 v1 scope; the pipeline-lag step and report step arrive in later
/// phases as analytics and reports exist). The item is deleted even when
/// verify fails or the journey is otherwise about to be reported as
/// failed -- best-effort cleanup, never left for the next run.
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

    let verified = time_step(
        metrics,
        "verify",
        verify_item(client, backend_url, item.id, &name, timeout),
    )
    .await;

    // Best-effort: attempted regardless of the verify outcome, and its
    // own result never changes the journey's success/failure verdict.
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
