package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/telemetry"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func TestTracingSpansAndPropagation(t *testing.T) {
	t.Parallel()
	var seen http.Header
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if bytes.Contains(body, []byte("ana.silva")) {
			t.Error("raw e-mail reached the upstream")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(
			[]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`),
		); err != nil {
			t.Error(err)
		}
	}))
	defer up.Close()
	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	kr, err := token.NewKeyring(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	key, err := kr.Tenant("acme")
	if err != nil {
		t.Fatal(err)
	}
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	srv := NewServer(
		slog.New(slog.DiscardHandler),
		telemetry.NewMetrics(),
		mask.NewEngine(detect.New(detect.Options{}), 0),
		[]*Tenant{{Mask: &mask.Tenant{ID: "acme", Key: key, Policy: policy.New(policy.Enforce, policy.Mask)}}},
		[]Route{{Name: "openai", Path: "/v1/chat/completions", Upstream: u, Codec: NewOpenAICodec()}},
		http.DefaultTransport,
		nil,
		Options{
			TenantHeader: "X-Maskwire-Key", SessionHeader: "X-Maskwire-Session", DefaultTenant: "acme",
			MaxBodyBytes: 1 << 20, Tracer: tp.Tracer("test"), Propagator: propagation.TraceContext{},
		},
	)
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"mail ana.silva@empresa.com.br"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Traceparent", parent)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	names := map[string]sdktrace.ReadOnlySpan{}
	for _, s := range rec.Ended() {
		names[s.Name()] = s
		if s.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("span %s did not join the incoming trace", s.Name())
		}
		for _, a := range s.Attributes() {
			if strings.Contains(a.Value.String(), "ana.silva") {
				t.Errorf("span %s attribute %s carries a raw value", s.Name(), a.Key)
			}
		}
	}
	for _, n := range []string{"maskwire openai", "mask", "upstream openai"} {
		if names[n] == nil {
			t.Fatalf("missing span %q, have %v", n, names)
		}
	}
	up2 := names["upstream openai"].SpanContext().SpanID().String()
	if tp := seen.Get("Traceparent"); !strings.Contains(tp, "4bf92f3577b34da6a3ce929d0e0e4736-"+up2) {
		t.Fatalf("upstream traceparent %q, want child span %s", tp, up2)
	}
	attrs := map[string]string{}
	for _, a := range names["mask"].Attributes() {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["maskwire.masked"] != "1" || attrs["maskwire.entity_types"] != "[\"EMAIL\"]" {
		t.Fatalf("mask attributes %v", attrs)
	}
}
