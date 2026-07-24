use prometheus::{
    CounterVec, Encoder, Gauge, Histogram, HistogramVec, Opts, Registry, TextEncoder,
};

/// Canary-exclusive metrics (ADR-0007 D9): journey outcome, per-step
/// latency (localizes which step is failing), staleness of the last
/// success, (v2) the pipeline-lag step's own lag and three-valued result,
/// and (v3) the report step's generation latency and four-valued result.
/// Each instance owns its own registry so tests can create independent
/// `Metrics` without colliding on Prometheus's global default registry.
pub struct Metrics {
    registry: Registry,
    pub journey_total: CounterVec,
    pub step_duration_seconds: HistogramVec,
    pub last_success_timestamp_seconds: Gauge,
    pub pipeline_lag_seconds: Histogram,
    pub pipeline_check_total: CounterVec,
    pub report_generation_seconds: Histogram,
    pub report_check_total: CounterVec,
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

        // Bounded label set: "step" is create|verify|pipeline|report|delete,
        // never free text.
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

        // Buckets span sub-second (fast ingestion) to the default
        // CANARY_PIPELINE_TIMEOUT_SECONDS (10s) -- only "ok" outcomes are
        // observed here (RFC-0001 D9 v2), so the histogram stays a clean
        // read of actual pipeline latency, not diluted by timeouts/skips.
        let pipeline_lag_seconds = Histogram::with_opts(
            prometheus::HistogramOpts::new(
                "canary_pipeline_lag_seconds",
                "End-to-end lag from item creation to first visibility in analytics",
            )
            .buckets(vec![0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 3.0, 5.0, 7.5, 10.0]),
        )
        .expect("valid pipeline_lag_seconds metric");

        // Bounded label set: "result" is ok|timeout|skipped, never free
        // text (Hard rule 9: none of the three fails the journey itself).
        let pipeline_check_total = CounterVec::new(
            Opts::new(
                "canary_pipeline_check_total",
                "Pipeline-lag polls against analytics, by result",
            ),
            &["result"],
        )
        .expect("valid pipeline_check_total metric");

        // Buckets span sub-second to the default CANARY_REPORT_TIMEOUT_SECONDS
        // (15s) -- a report is a slower async POI/PDF render than an analytics
        // read, so the tail runs longer than pipeline_lag_seconds. Only "ok"
        // outcomes are observed here (RFC-0001 D9 v3), like pipeline_lag, so
        // the histogram stays a clean read of real generation latency, not
        // diluted by timeouts/failures/skips.
        let report_generation_seconds = Histogram::with_opts(
            prometheus::HistogramOpts::new(
                "canary_report_generation_seconds",
                "End-to-end report generation latency, submit to SUCCEEDED",
            )
            .buckets(vec![0.1, 0.25, 0.5, 1.0, 2.0, 3.0, 5.0, 7.5, 10.0, 15.0]),
        )
        .expect("valid report_generation_seconds metric");

        // Bounded label set: "result" is ok|timeout|failed|skipped, never free
        // text (Hard rule 9: none of the four fails the journey itself).
        let report_check_total = CounterVec::new(
            Opts::new(
                "canary_report_check_total",
                "Report steps against reports, by result",
            ),
            &["result"],
        )
        .expect("valid report_check_total metric");

        registry
            .register(Box::new(journey_total.clone()))
            .expect("register journey_total");
        registry
            .register(Box::new(step_duration_seconds.clone()))
            .expect("register step_duration_seconds");
        registry
            .register(Box::new(last_success_timestamp_seconds.clone()))
            .expect("register last_success_timestamp_seconds");
        registry
            .register(Box::new(pipeline_lag_seconds.clone()))
            .expect("register pipeline_lag_seconds");
        registry
            .register(Box::new(pipeline_check_total.clone()))
            .expect("register pipeline_check_total");
        registry
            .register(Box::new(report_generation_seconds.clone()))
            .expect("register report_generation_seconds");
        registry
            .register(Box::new(report_check_total.clone()))
            .expect("register report_check_total");

        Self {
            registry,
            journey_total,
            step_duration_seconds,
            last_success_timestamp_seconds,
            pipeline_lag_seconds,
            pipeline_check_total,
            report_generation_seconds,
            report_check_total,
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
