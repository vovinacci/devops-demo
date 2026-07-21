package ingest

import (
	"context"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// metadataCarrier adapts grpc/metadata.MD to propagation.TextMapCarrier
// so the global W3C propagator (internal/otelsetup) can inject the active
// span's trace context into outgoing gRPC metadata. The scaffold has no
// otelgrpc dependency (go.opentelemetry.io/contrib/instrumentation/
// google.golang.org/grpc/otelgrpc) and this client only ever originates
// two RPC shapes (one unary, one server-stream) -- a manual carrier plus
// two small interceptors is less new surface than pulling in and
// configuring a whole instrumentation package for it.
type metadataCarrier struct{ md *metadata.MD }

func (c metadataCarrier) Get(key string) string {
	vals := c.md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c metadataCarrier) Set(key, value string) { c.md.Set(key, value) }

func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.md))
	for k := range *c.md {
		keys = append(keys, k)
	}
	return keys
}

func injectTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	otel.GetTextMapPropagator().Inject(ctx, metadataCarrier{&md})
	return metadata.NewOutgoingContext(ctx, md)
}

// traceUnaryClientInterceptor injects the caller's span context (the
// per-cycle ingest span, see client.go) into the ListItems call.
func traceUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		return invoker(injectTraceContext(ctx), method, req, reply, cc, opts...)
	}
}

// traceStreamClientInterceptor injects the same span context into the
// WatchItemEvents stream open call. The stream itself carries no
// per-event context (the proto has none); events are correlated to the
// per-cycle span via structured logs instead (client.go).
func traceStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(injectTraceContext(ctx), desc, cc, method, opts...)
	}
}
