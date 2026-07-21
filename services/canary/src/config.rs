use std::env;
use std::time::Duration;

/// Runtime configuration, read once at startup from the environment
/// (uniform service contract, RFC-0001 D6). Every value has a default so
/// the canary runs unconfigured in a pinch.
pub struct Config {
    pub backend_url: String,
    pub interval: Duration,
    pub timeout: Duration,
}

impl Config {
    pub fn from_env() -> Self {
        Self {
            backend_url: env_string("CANARY_BACKEND_URL", "http://api:8000"),
            interval: Duration::from_secs(env_u64("CANARY_INTERVAL_SECONDS", 30)),
            timeout: Duration::from_secs(env_u64("CANARY_TIMEOUT_SECONDS", 5)),
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
