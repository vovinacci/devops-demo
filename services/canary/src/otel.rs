use opentelemetry::global;
use opentelemetry::propagation::Injector;
use opentelemetry::trace::{TraceContextExt, TracerProvider as _};
use opentelemetry_sdk::propagation::TraceContextPropagator;
use opentelemetry_sdk::trace::{SdkTracerProvider, SpanData, SpanExporter};
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use tracing_opentelemetry::OpenTelemetrySpanExt;
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;
use tracing_subscriber::{EnvFilter, Registry};

/// Discards every span instead of shipping it anywhere (RFC-0001 D11,
/// ADR-0010): OpenTelemetry is wired end to end -- ID generation, context
/// propagation, tracing integration -- but the trace *backend* (Tempo) is
/// deferred, so exported spans would have nowhere to go. Trace IDs still
/// land in structured logs (see `journey::run` and the `fmt` JSON layer
/// below); dropping each span here (via `with_simple_exporter`, which
/// calls `export` synchronously per span -- there is no batching to
/// drop) is the "no exporter configured" half of that decision, not a
/// bug.
#[derive(Debug, Default)]
struct NullSpanExporter;

impl SpanExporter for NullSpanExporter {
    async fn export(&self, _batch: Vec<SpanData>) -> opentelemetry_sdk::error::OTelSdkResult {
        Ok(())
    }
}

/// Initializes structured JSON logging (uniform service contract, D6) and
/// OpenTelemetry tracing (D11). Returns the `SdkTracerProvider` -- it must
/// stay alive for the process lifetime, or spans stop being recorded the
/// moment it is dropped.
pub fn init() -> SdkTracerProvider {
    global::set_text_map_propagator(TraceContextPropagator::new());

    let provider = SdkTracerProvider::builder()
        .with_simple_exporter(NullSpanExporter)
        .build();
    let tracer = provider.tracer("canary");

    let otel_layer = tracing_opentelemetry::layer().with_tracer(tracer);
    let fmt_layer = tracing_subscriber::fmt::layer()
        .json()
        .with_current_span(true)
        .with_span_list(false);
    let env_filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

    Registry::default()
        .with(env_filter)
        .with(fmt_layer)
        .with(otel_layer)
        .init();

    provider
}

/// The W3C trace ID of the current span, for explicit log correlation
/// (Tempo is deferred, so this is the correlation mechanism today).
pub fn current_trace_id() -> String {
    tracing::Span::current()
        .context()
        .span()
        .span_context()
        .trace_id()
        .to_string()
}

struct HeaderInjector<'a>(&'a mut HeaderMap);

impl Injector for HeaderInjector<'_> {
    fn set(&mut self, key: &str, value: String) {
        if let (Ok(name), Ok(val)) = (
            HeaderName::from_bytes(key.as_bytes()),
            HeaderValue::from_str(&value),
        ) {
            self.0.insert(name, val);
        }
    }
}

/// Injects the current span's W3C `traceparent` (and `tracestate`, if any)
/// into an outbound request's headers, per D11.
pub fn inject_traceparent(headers: &mut HeaderMap) {
    let cx = tracing::Span::current().context();
    global::get_text_map_propagator(|propagator| {
        propagator.inject_context(&cx, &mut HeaderInjector(headers));
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_sdk::trace::{RandomIdGenerator, SdkTracerProvider};

    /// Guards against a regression this module already hit once: without
    /// an active span (or without a tracer provider installed), the W3C
    /// propagator has nothing to inject and outbound calls silently carry
    /// no trace context.
    #[test]
    fn inject_traceparent_sets_a_real_w3c_header() {
        global::set_text_map_propagator(TraceContextPropagator::new());
        let provider = SdkTracerProvider::builder()
            .with_simple_exporter(NullSpanExporter)
            .with_id_generator(RandomIdGenerator::default())
            .build();
        let tracer = provider.tracer("test");
        let otel_layer = tracing_opentelemetry::layer().with_tracer(tracer);
        let _guard = tracing_subscriber::registry()
            .with(otel_layer)
            .set_default();

        let span = tracing::info_span!("test_span");
        let _entered = span.enter();

        let mut headers = HeaderMap::new();
        inject_traceparent(&mut headers);

        let traceparent = headers
            .get("traceparent")
            .expect("traceparent header set")
            .to_str()
            .unwrap();
        // 00-<32 hex trace id>-<16 hex span id>-<flags>
        assert!(
            traceparent.starts_with("00-"),
            "unexpected traceparent format: {traceparent}"
        );
        assert_ne!(
            &traceparent[3..35],
            "00000000000000000000000000000000",
            "trace id must not be all zeros: {traceparent}"
        );
    }
}
