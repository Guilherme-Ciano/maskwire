package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const TracerName = "github.com/Guilherme-Ciano/maskwire"

type TracingOptions struct {
	Endpoint    string
	SampleRatio float64
	Version     string
}

type Tracing struct {
	Provider   trace.TracerProvider
	Propagator propagation.TextMapPropagator
	shutdown   func(context.Context) error
}

func (t *Tracing) Tracer() trace.Tracer {
	return t.Provider.Tracer(TracerName)
}

func (t *Tracing) Enabled() trace.Tracer {
	if t.shutdown == nil {
		return nil
	}
	return t.Tracer()
}

func (t *Tracing) Shutdown(ctx context.Context) error {
	if t.shutdown == nil {
		return nil
	}
	if err := t.shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracing: %w", err)
	}
	return nil
}

func tracesURL(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%w: %q", ErrEndpoint, endpoint)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	return u.String(), nil
}

var ErrEndpoint = errors.New("tracing endpoint must be an http or https URL")

var ErrSampleRatio = errors.New("tracing sample ratio must be in 0..1")

func NewTracing(ctx context.Context, o TracingOptions) (*Tracing, error) {
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	if o.Endpoint == "" {
		return &Tracing{Provider: noop.NewTracerProvider(), Propagator: prop}, nil
	}
	if o.SampleRatio < 0 || o.SampleRatio > 1 {
		return nil, ErrSampleRatio
	}
	endpoint, err := tracesURL(o.Endpoint)
	if err != nil {
		return nil, err
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName("maskwire"), semconv.ServiceVersion(o.Version))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.SampleRatio))),
	)
	return &Tracing{Provider: tp, Propagator: prop, shutdown: tp.Shutdown}, nil
}
