use std::sync::Arc;
use std::sync::atomic::Ordering;

use canary::config::Config;
use canary::http::{self, AppState};
use canary::journey::PipelineConfig;
use canary::{journey, metrics, otel};
use tokio::signal;

#[tokio::main]
async fn main() {
    // Kept alive for the process lifetime: dropping it stops span export
    // (see otel::init doc comment).
    let _tracer_provider = otel::init();

    let config = Config::from_env();
    let state = Arc::new(AppState {
        metrics: metrics::Metrics::new(),
        ready: std::sync::atomic::AtomicBool::new(false),
    });

    tokio::spawn(run_journey_loop(state.clone(), config));

    let app = http::router(state);
    let listener = tokio::net::TcpListener::bind("0.0.0.0:8080")
        .await
        .expect("bind 0.0.0.0:8080");
    tracing::info!("canary listening on :8080");
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await
        .expect("axum serve");
}

/// Runs the journey on a fixed interval. Readiness flips true as soon as
/// this loop starts (RFC-0001 D6/D10): the canary is ready to do its job
/// regardless of whether the backend it monitors is currently up.
async fn run_journey_loop(state: Arc<AppState>, config: Config) {
    let client = reqwest::Client::new();
    state.ready.store(true, Ordering::Relaxed);

    let pipeline = PipelineConfig {
        analytics_url: &config.analytics_url,
        timeout: config.pipeline_timeout,
        poll_interval: config.pipeline_poll_interval,
    };

    let mut ticker = tokio::time::interval(config.interval);
    loop {
        ticker.tick().await;
        journey::run(
            &client,
            &config.backend_url,
            config.timeout,
            &state.metrics,
            &pipeline,
        )
        .await;
    }
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c().await.expect("install ctrl_c handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("install SIGTERM handler")
            .recv()
            .await;
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
    tracing::info!("shutdown signal received, draining connections");
}
