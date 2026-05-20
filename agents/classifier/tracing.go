package main

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/nats-io/nats.go"
)

// natsCarrier bridges nats.Header to the OTel TextMapCarrier interface.
type natsCarrier struct{ h nats.Header }

func (c natsCarrier) Get(key string) string { return c.h.Get(key) }
func (c natsCarrier) Set(key, val string)   { c.h.Set(key, val) }
func (c natsCarrier) Keys() []string {
	keys := make([]string, 0, len(c.h))
	for k := range c.h {
		keys = append(keys, k)
	}
	return keys
}

// extractCtx extracts W3C trace context from an incoming NATS message's headers.
func extractCtx(msg *nats.Msg) context.Context {
	if msg.Header == nil {
		return context.Background()
	}
	return otel.GetTextMapPropagator().Extract(context.Background(), natsCarrier{msg.Header})
}

// injectHeaders writes W3C traceparent plus explicit traceId/spanId headers so
// downstream agents can continue the same trace and operators can grep headers directly.
func injectHeaders(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}
	otel.GetTextMapPropagator().Inject(ctx, natsCarrier{msg.Header})
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		msg.Header.Set("traceId", sc.TraceID().String())
		msg.Header.Set("spanId", sc.SpanID().String())
	}
}

// initTracer wires up the global OTLP HTTP tracer provider.
// Returns a shutdown function that must be deferred in main.
func initTracer(ctx context.Context) (func(context.Context) error, error) {
	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = "agent"
	}

	ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if ep == "" {
		ep = "http://localhost:4318"
	}
	// WithEndpoint expects host:port — strip scheme and any path.
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	if i := strings.IndexByte(ep, '/'); i != -1 {
		ep = ep[:i]
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(ep),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", svcName),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
