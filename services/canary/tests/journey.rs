//! Journey logic against a mock backend (wiremock) -- no real network.
//! Covers the three outcomes the runbook (docs/runbooks/canary-journey-failing.md)
//! and the CanaryJourneyFailing alert are built around: success, a
//! correctness gap (verify miss), and a reachability gap (backend down).

use std::sync::{Arc, Mutex};
use std::time::Duration;

use canary::journey::{self, Outcome};
use canary::metrics::Metrics;
use reqwest::Client;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, Request, ResponseTemplate};

const TIMEOUT: Duration = Duration::from_secs(5);
const ITEM_ID: i64 = 1;

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
    let outcome = journey::run(&client, &server.uri(), TIMEOUT, &metrics).await;

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
    let outcome = journey::run(&client, &server.uri(), TIMEOUT, &metrics).await;

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
    // Guaranteed-unreachable address: bind then immediately drop, so the
    // port is free but nothing answers on it. No wiremock server involved
    // -- this exercises the "reachability" failure mode, not a mocked one.
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind ephemeral port");
    let backend_url = format!("http://127.0.0.1:{}", listener.local_addr().unwrap().port());
    drop(listener);

    let metrics = Metrics::new();
    let client = Client::new();
    let outcome = journey::run(&client, &backend_url, TIMEOUT, &metrics).await;

    assert_eq!(outcome, Outcome::Failure);
    assert_eq!(
        metrics.journey_total.with_label_values(&["failure"]).get(),
        1.0
    );
    // Only the create step ran; there is nothing to verify or delete.
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
            .with_label_values(&["delete"])
            .get_sample_count(),
        0
    );
}
