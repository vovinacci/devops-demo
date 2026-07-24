use std::env;
use std::time::Duration;

/// Runtime configuration, read once at startup from the environment
/// (uniform service contract, RFC-0001 D6). Every value has a default so
/// the canary runs unconfigured in a pinch.
pub struct Config {
    pub backend_url: String,
    pub interval: Duration,
    pub timeout: Duration,
    /// Base URL of analytics for the v2 pipeline-lag step (RFC-0001 D9).
    /// Analytics is an additive compose profile (ADR-0008 D10): the
    /// `synthetic` profile must run with it absent, so this URL is only
    /// ever *attempted*, never depended on.
    pub analytics_url: String,
    /// Overall budget for the pipeline-lag poll loop, measured from when
    /// the step starts polling (RFC-0001 D9 v2).
    pub pipeline_timeout: Duration,
    /// Delay between poll attempts within `pipeline_timeout`.
    pub pipeline_poll_interval: Duration,
    /// Base URL of reports for the v3 report step (RFC-0001 D9). Reports is
    /// an additive compose profile (ADR-0008 D10), exactly like analytics:
    /// the `synthetic` profile must run with it absent, so this URL is only
    /// ever *attempted*, never depended on -- unreachable is a normal
    /// `skipped` outcome, not a startup failure.
    pub reports_url: String,
    /// Overall budget for the report step -- larger than the pipeline
    /// timeout because a report is an async POI/PDF render, slower than an
    /// analytics read (RFC-0001 D2).
    pub report_timeout: Duration,
    /// Delay between report status polls within `report_timeout`.
    pub report_poll_interval: Duration,
}

impl Config {
    pub fn from_env() -> Self {
        Self {
            backend_url: env_string("CANARY_BACKEND_URL", "http://api:8000"),
            interval: Duration::from_secs(env_u64("CANARY_INTERVAL_SECONDS", 30)),
            timeout: Duration::from_secs(env_u64("CANARY_TIMEOUT_SECONDS", 5)),
            analytics_url: env_string("CANARY_ANALYTICS_URL", "http://analytics:8082"),
            pipeline_timeout: Duration::from_secs(env_u64("CANARY_PIPELINE_TIMEOUT_SECONDS", 10)),
            pipeline_poll_interval: Duration::from_millis(env_u64(
                "CANARY_PIPELINE_POLL_INTERVAL_MS",
                250,
            )),
            reports_url: env_string("CANARY_REPORTS_URL", "http://reports:8083"),
            report_timeout: Duration::from_secs(env_u64("CANARY_REPORT_TIMEOUT_SECONDS", 15)),
            report_poll_interval: Duration::from_millis(env_u64(
                "CANARY_REPORT_POLL_INTERVAL_MS",
                500,
            )),
        }
    }
}

fn env_string(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_u64(key: &str, default: u64) -> u64 {
    env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}
