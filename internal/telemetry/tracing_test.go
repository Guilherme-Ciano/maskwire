package telemetry

import (
	"errors"
	"testing"
)

func TestTracesURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://jaeger:4318":            "http://jaeger:4318/v1/traces",
		"http://jaeger:4318/":           "http://jaeger:4318/v1/traces",
		"https://otel.example/custom/x": "https://otel.example/custom/x",
	} {
		got, err := tracesURL(in)
		if err != nil || got != want {
			t.Errorf("%s: got %s, %v", in, got, err)
		}
	}
	for _, bad := range []string{"jaeger:4318", "ftp://x", "http://"} {
		if _, err := tracesURL(bad); !errors.Is(err, ErrEndpoint) {
			t.Errorf("%s accepted: %v", bad, err)
		}
	}
}

func TestNewTracing(t *testing.T) {
	off, err := NewTracing(t.Context(), TracingOptions{})
	if err != nil || off.Tracer() == nil || off.Shutdown(t.Context()) != nil {
		t.Fatalf("disabled tracing: %v", err)
	}
	if _, err := NewTracing(
		t.Context(),
		TracingOptions{Endpoint: "http://x:4318", SampleRatio: 2},
	); !errors.Is(
		err,
		ErrSampleRatio,
	) {
		t.Fatalf("ratio: %v", err)
	}
	on, err := NewTracing(t.Context(), TracingOptions{Endpoint: "http://127.0.0.1:1", SampleRatio: 1, Version: "t"})
	if err != nil {
		t.Fatal(err)
	}
	_, span := on.Tracer().Start(t.Context(), "x")
	span.End()
	if err := on.Shutdown(t.Context()); err == nil {
		t.Log("exporter flushed to an unreachable endpoint without error")
	}
}
