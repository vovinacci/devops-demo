use prometheus::{CounterVec, Encoder, Gauge, HistogramVec, Opts, Registry, TextEncoder};

/// Canary-exclusive metrics (ADR-0007 D9): journey outcome, per-step
/// latency (localizes which step is failing), and staleness of the last
/// success. Each instance owns its own registry so tests can create
/// independent `Metrics` without colliding on Prometheus's global default
/// registry.
pub struct Metrics {
    registry: Registry,
    pub journey_total: CounterVec,
    pub step_duration_seconds: HistogramVec,
    pub last_success_timestamp_seconds: Gauge,
}

impl Metrics {
    pub fn new() -> Self {
        let registry = Registry::new();

        // Bounded label set: "result" is success|failure, never free text.
        let journey_total = CounterVec::new(
            Opts::new("canary_journey_total", "Synthetic journeys, by outcome"),
            &["result"],
        )
        .expect("valid journey_total metric");

        // Bounded label set: "step" is create|verify|delete, never free text.
        let step_duration_seconds = HistogramVec::new(
            prometheus::HistogramOpts::new(
                "canary_journey_step_duration_seconds",
                "Duration of each journey step",
            ),
            &["step"],
        )
        .expect("valid step_duration_seconds metric");

        let last_success_timestamp_seconds = Gauge::new(
            "canary_journey_last_success_timestamp_seconds",
            "Unix timestamp of the last fully successful journey",
        )
        .expect("valid last_success_timestamp_seconds metric");

        registry
            .register(Box::new(journey_total.clone()))
            .expect("register journey_total");
        registry
            .register(Box::new(step_duration_seconds.clone()))
            .expect("register step_duration_seconds");
        registry
            .register(Box::new(last_success_timestamp_seconds.clone()))
            .expect("register last_success_timestamp_seconds");

        Self {
            registry,
            journey_total,
            step_duration_seconds,
            last_success_timestamp_seconds,
        }
    }

    /// Render in Prometheus text exposition format for the `/metrics`
    /// handler. Fallible: a scrape must not be able to crash the process,
    /// so encode errors are returned to the caller (-> HTTP 500) instead
    /// of panicking.
    pub fn encode(&self) -> Result<Vec<u8>, prometheus::Error> {
        let families = self.registry.gather();
        let mut buf = Vec::new();
        TextEncoder::new().encode(&families, &mut buf)?;
        Ok(buf)
    }
}

impl Default for Metrics {
    fn default() -> Self {
        Self::new()
    }
}
