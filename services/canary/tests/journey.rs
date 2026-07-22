//! Journey logic against a mock backend (wiremock) -- no real network.
//! Covers the three outcomes the runbook (docs/runbooks/canary-journey-failing.md)
//! and the CanaryJourneyFailing alert are built around: success, a
//! correctness gap (verify miss), and a reachability gap (backend down).

use std::sync::{Arc, Mutex};
use std::time::Duration;

use canary::journey::{self, Outcome, PipelineConfig};
use canary::metrics::Metrics;
use reqwest::Client;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, Request, ResponseTemplate};

const TIMEOUT: Duration = Duration::from_secs(5);
const ITEM_ID: i64 = 1;

/// A closed ephemeral port -- reachable enough to bind, guaranteed to
/// refuse the connection immediately after. Used by tests that need a
/// fast, guaranteed-unreachable analytics URL (mirrors the
/// `backend_down_is_a_failure...` pattern below).
fn unreachable_url() -> String {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind ephemeral port");
    let url = format!("http://127.0.0.1:{}", listener.local_addr().unwrap().port());
    drop(listener);
    url
}

/// The existing (pre-v2) tests don't exercise the pipeline-lag step
/// itself, so they point it at a closed port -- it resolves to `skipped`
/// on the first poll and adds no meaningful delay.
fn skipped_pipeline_config(analytics_url: &str) -> PipelineConfig<'_> {
    PipelineConfig {
        analytics_url,
        timeout: Duration::from_secs(1),
        poll_interval: Duration::from_millis(50),
    }
}

type CreatedName = Arc<Mutex<Option<String>>>;

fn create_responder(created: CreatedName) -> impl Fn(&Request) -> ResponseTemplate {
    move |request: &Request| {
        let body: Value = request.body_json().expect("valid JSON body");
        let name = body["name"].as_str().expect("name field").to_string();
        *created.lock().unwrap() = Some(name.clone());
        ResponseTemplate::new(201).set_body_json(json!({"id": ITEM_ID, "name": name}))
    }
}

fn list_responder(
    created: CreatedName,
    include_item: bool,
) -> impl Fn(&Request) -> ResponseTemplate {
    move |_request: &Request| {
        let items: Vec<Value> = if include_item {
            match created.lock().unwrap().clone() {
                Some(name) => vec![json!({"id": ITEM_ID, "name": name})],
                None => vec![],
            }
        } else {
            vec![]
        };
        ResponseTemplate::new(200).set_body_json(items)
    }
}

async fn delete_count(server: &MockServer) -> usize {
    server
        .received_requests()
        .await
        .expect("request recording enabled")
        .iter()
        .filter(|r| r.method == reqwest::Method::DELETE)
        .count()
}

#[tokio::test]
async fn success_path_records_success_and_cleans_up() {
    let server = MockServer::start().await;
    let created: CreatedName = Arc::new(Mutex::new(None));

    Mock::given(method("POST"))
        .and(path("/items"))
        .respond_with(create_responder(created.clone()))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(list_responder(created.clone(), true))
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path(format!("/items/{ITEM_ID}")))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    let metrics = Metrics::new();
    let client = Client::new();
    let analytics_url = unreachable_url();
    let outcome = journey::run(
        &client,
        &server.uri(),
        TIMEOUT,
        &metrics,
        &skipped_pipeline_config(&analytics_url),
    )
    .await;

    assert_eq!(outcome, Outcome::Success);
    assert_eq!(
        metrics.journey_total.with_label_values(&["success"]).get(),
        1.0
    );
    assert_eq!(
        metrics.journey_total.with_label_values(&["failure"]).get(),
        0.0
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["create"])
            .get_sample_count(),
        1
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["verify"])
            .get_sample_count(),
        1
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["delete"])
            .get_sample_count(),
        1
    );
    assert!(metrics.last_success_timestamp_seconds.get() > 0.0);
    assert_eq!(
        delete_count(&server).await,
        1,
        "cleanup must run on success"
    );
}

#[tokio::test]
async fn verify_miss_is_a_failure_but_still_cleans_up() {
    let server = MockServer::start().await;
    let created: CreatedName = Arc::new(Mutex::new(None));

    Mock::given(method("POST"))
        .and(path("/items"))
        .respond_with(create_responder(created.clone()))
        .mount(&server)
        .await;
    // The item is never visible in the list -- a correctness gap, not a
    // reachability one (D9: this is exactly what the synthetic layer
    // catches and whitebox/blackbox cannot).
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(list_responder(created.clone(), false))
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path(format!("/items/{ITEM_ID}")))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    let metrics = Metrics::new();
    let client = Client::new();
    let analytics_url = unreachable_url();
    let outcome = journey::run(
        &client,
        &server.uri(),
        TIMEOUT,
        &metrics,
        &skipped_pipeline_config(&analytics_url),
    )
    .await;

    assert_eq!(outcome, Outcome::Failure);
    assert_eq!(
        metrics.journey_total.with_label_values(&["failure"]).get(),
        1.0
    );
    assert_eq!(
        metrics.journey_total.with_label_values(&["success"]).get(),
        0.0
    );
    assert_eq!(metrics.last_success_timestamp_seconds.get(), 0.0);
    assert_eq!(
        delete_count(&server).await,
        1,
        "cleanup must run even when verify fails"
    );
}

#[tokio::test]
async fn backend_down_is_a_failure_with_no_cleanup_attempt() {
    // Guaranteed-unreachable address -- this exercises the "reachability"
    // failure mode, not a mocked one. No wiremock server involved.
    let backend_url = unreachable_url();
    // Create fails before the pipeline-lag step is ever reached, so this
    // URL is never dialed; kept unreachable anyway so the test would
    // still be fast if that assumption ever changes.
    let analytics_url = unreachable_url();

    let metrics = Metrics::new();
    let client = Client::new();
    let outcome = journey::run(
        &client,
        &backend_url,
        TIMEOUT,
        &metrics,
        &skipped_pipeline_config(&analytics_url),
    )
    .await;

    assert_eq!(outcome, Outcome::Failure);
    assert_eq!(
        metrics.journey_total.with_label_values(&["failure"]).get(),
        1.0
    );
    // Only the create step ran; there is nothing to verify, pipeline-lag,
    // or delete.
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["create"])
            .get_sample_count(),
        1
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["verify"])
            .get_sample_count(),
        0
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["pipeline"])
            .get_sample_count(),
        0
    );
    assert_eq!(
        metrics
            .step_duration_seconds
            .with_label_values(&["delete"])
            .get_sample_count(),
        0
    );
}

// -- Pipeline-lag step (RFC-0001 D9 v2, ADR-0008 D10) --
//
// These mount a second `MockServer` standing in for analytics, separate
// from the backend server, so create/verify/delete and the pipeline poll
// never share mocks or paths.

async fn backend_mocks(server: &MockServer, created: CreatedName) {
    Mock::given(method("POST"))
        .and(path("/items"))
        .respond_with(create_responder(created.clone()))
        .mount(server)
        .await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(list_responder(created.clone(), true))
        .mount(server)
        .await;
    Mock::given(method("DELETE"))
        .and(path(format!("/items/{ITEM_ID}")))
        .respond_with(ResponseTemplate::new(204))
        .mount(server)
        .await;
}

#[tokio::test]
async fn pipeline_lag_ok_records_lag_and_result() {
    let backend = MockServer::start().await;
    let analytics = MockServer::start().await;
    let created: CreatedName = Arc::new(Mutex::new(None));
    backend_mocks(&backend, created.clone()).await;

    // 404 twice (not yet ingested), then 200 (visible) -- wiremock serves
    // mounted mocks in most-recently-mounted-first order, so mounting the
    // limited 404 mock after building on top of an always-200 base would
    // invert that; instead this uses an explicit call counter so ordering
    // is exact regardless of mount order.
    let poll_count = Arc::new(Mutex::new(0u32));
    Mock::given(method("GET"))
        .and(path(format!("/api/v1/items/{ITEM_ID}")))
        .respond_with(move |_req: &Request| {
            let mut count = poll_count.lock().unwrap();
            *count += 1;
            if *count <= 2 {
                ResponseTemplate::new(404)
            } else {
                ResponseTemplate::new(200).set_body_json(json!({
                    "item_id": ITEM_ID,
                    "name": "irrelevant",
                    "first_seen": "2026-01-01T00:00:00Z",
                    "last_seen": "2026-01-01T00:00:00Z",
                    "tombstoned": false
                }))
            }
        })
        .mount(&analytics)
        .await;

    let metrics = Metrics::new();
    let client = Client::new();
    let pipeline = PipelineConfig {
        analytics_url: &analytics.uri(),
        timeout: Duration::from_secs(2),
        poll_interval: Duration::from_millis(20),
    };
    let outcome = journey::run(&client, &backend.uri(), TIMEOUT, &metrics, &pipeline).await;

    assert_eq!(
        outcome,
        Outcome::Success,
        "pipeline outcome never fails the journey"
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["ok"])
            .get(),
        1.0
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["timeout"])
            .get(),
        0.0
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["skipped"])
            .get(),
        0.0
    );
    assert_eq!(metrics.pipeline_lag_seconds.get_sample_count(), 1);
    assert!(metrics.pipeline_lag_seconds.get_sample_sum() >= 0.0);
}

#[tokio::test]
async fn pipeline_lag_timeout_never_fails_the_journey() {
    let backend = MockServer::start().await;
    let analytics = MockServer::start().await;
    let created: CreatedName = Arc::new(Mutex::new(None));
    backend_mocks(&backend, created.clone()).await;

    // Analytics is reachable but the item never becomes visible -- the
    // pipeline-lag signal firing (RFC-0001 D9 v2's whole reason to exist).
    Mock::given(method("GET"))
        .and(path(format!("/api/v1/items/{ITEM_ID}")))
        .respond_with(ResponseTemplate::new(404))
        .mount(&analytics)
        .await;

    let metrics = Metrics::new();
    let client = Client::new();
    let pipeline = PipelineConfig {
        analytics_url: &analytics.uri(),
        timeout: Duration::from_millis(200),
        poll_interval: Duration::from_millis(50),
    };
    let outcome = journey::run(&client, &backend.uri(), TIMEOUT, &metrics, &pipeline).await;

    assert_eq!(
        outcome,
        Outcome::Success,
        "Hard rule 9: a stuck pipeline-lag step must not fail the journey"
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["timeout"])
            .get(),
        1.0
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["ok"])
            .get(),
        0.0
    );
    assert_eq!(metrics.pipeline_lag_seconds.get_sample_count(), 0);
    assert_eq!(
        delete_count(&backend).await,
        1,
        "cleanup still runs after a pipeline timeout"
    );
}

#[tokio::test]
async fn pipeline_lag_skipped_when_analytics_is_unreachable() {
    let backend = MockServer::start().await;
    let created: CreatedName = Arc::new(Mutex::new(None));
    backend_mocks(&backend, created.clone()).await;

    // Analytics profile not up (ADR-0008 D10): closed port, connection
    // refused on the very first poll.
    let analytics_url = unreachable_url();

    let metrics = Metrics::new();
    let client = Client::new();
    let pipeline = PipelineConfig {
        analytics_url: &analytics_url,
        // Deliberately long relative to how fast this test must run: if
        // the skip path degraded into polling-until-timeout, this test
        // would take 30s and fail the "fast" assertion below well before
        // that.
        timeout: Duration::from_secs(30),
        poll_interval: Duration::from_millis(50),
    };

    let start = std::time::Instant::now();
    let outcome = journey::run(&client, &backend.uri(), TIMEOUT, &metrics, &pipeline).await;
    let elapsed = start.elapsed();

    assert_eq!(outcome, Outcome::Success);
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["skipped"])
            .get(),
        1.0
    );
    assert_eq!(
        metrics
            .pipeline_check_total
            .with_label_values(&["timeout"])
            .get(),
        0.0
    );
    assert_eq!(metrics.pipeline_lag_seconds.get_sample_count(), 0);
    assert!(
        elapsed < Duration::from_secs(5),
        "skip must be detected fast, not by waiting out the 30s timeout: took {elapsed:?}"
    );
    assert_eq!(
        delete_count(&backend).await,
        1,
        "cleanup still runs after a skipped pipeline check"
    );
}
