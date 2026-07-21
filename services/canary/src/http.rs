use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use axum::Router;
use axum::extract::State;
use axum::http::{StatusCode, header};
use axum::response::IntoResponse;
use axum::routing::get;

use crate::metrics::Metrics;

pub struct AppState {
    pub metrics: Metrics,
    /// Set once the journey loop has started. The canary's dependency
    /// (the backend it journeys against) is the thing it *monitors* --
    /// per RFC-0001 D10 graceful degradation, a down backend must not
    /// make the canary itself unready. See README "Readiness semantics".
    pub ready: AtomicBool,
}

pub fn router(state: Arc<AppState>) -> Router {
    Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .route("/metrics", get(metrics_handler))
        .with_state(state)
}

async fn healthz() -> StatusCode {
    StatusCode::OK
}

async fn readyz(State(state): State<Arc<AppState>>) -> StatusCode {
    if state.ready.load(Ordering::Relaxed) {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}

async fn metrics_handler(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    match state.metrics.encode() {
        Ok(body) => (
            StatusCode::OK,
            [(header::CONTENT_TYPE, "text/plain; version=0.0.4")],
            body,
        )
            .into_response(),
        Err(err) => {
            tracing::error!(error = %err, "failed to encode metrics");
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}
