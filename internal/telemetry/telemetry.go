package telemetry

import (
	"io"
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	labelTenant    = "tenant"
	labelStreaming = "streaming"
	labelRoute     = "route"
	labelType      = "type"
)

func redact(_ []string, a slog.Attr) slog.Attr {
	switch strings.ToLower(a.Key) {
	case "authorization", "api_key", "cookie", "prompt", "completion", "body", "content", "text", "value", "original":
		return slog.String(a.Key, "[redacted]")
	}
	return a
}

func NewLogger(w io.Writer, level, format string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: redact}
	if format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

type Metrics struct {
	Registry       *prometheus.Registry
	Requests       *prometheus.CounterVec
	Detections     *prometheus.CounterVec
	Blocked        *prometheus.CounterVec
	ForeignTokens  *prometheus.CounterVec
	Restores       *prometheus.CounterVec
	ToolWithheld   *prometheus.CounterVec
	MaskDuration   *prometheus.HistogramVec
	RequestLatency *prometheus.HistogramVec
	Overhead       *prometheus.HistogramVec
	Reloads        *prometheus.CounterVec
	Holdback       prometheus.Histogram
	StreamEvents   *prometheus.CounterVec

	ResponseDetections *prometheus.CounterVec
}

func durationBuckets() []float64 {
	return []float64{
		0.00001,
		0.000025,
		0.00005,
		0.0001,
		0.00025,
		0.0005,
		0.001,
		0.0025,
		0.005,
		0.01,
		0.025,
		0.05,
		0.1,
		0.25,
		0.5,
		1,
		2.5,
		5,
		10,
		30,
	}
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		Registry: reg,
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_requests_total", Help: "Proxied requests by tenant, route and status code.",
		}, []string{labelTenant, labelRoute, "status"}),
		Detections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_detections_total", Help: "Detected entities by tenant, type and applied action.",
		}, []string{labelTenant, labelType, "action"}),
		Blocked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_blocked_requests_total", Help: "Requests rejected by a block policy.",
		}, []string{labelTenant, labelType}),
		ForeignTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_foreign_tokens_total",
			Help: "Well-formed tokens in responses that were not issued for the request.",
		}, []string{labelTenant, labelType}),
		Restores: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_restores_total", Help: "Tokens restored in responses.",
		}, []string{labelTenant, labelStreaming}),
		ToolWithheld: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_tool_withheld_total",
			Help: "Tokens left masked in tool-call arguments by the tenant's tool_restore policy.",
		}, []string{labelTenant}),
		MaskDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "maskwire_mask_duration_seconds", Help: "Time spent detecting and masking a request body.",
			Buckets: durationBuckets(), NativeHistogramBucketFactor: 1.1,
		}, []string{labelRoute}),
		RequestLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "maskwire_request_duration_seconds", Help: "End-to-end request duration including upstream.",
			Buckets: durationBuckets(), NativeHistogramBucketFactor: 1.1,
		}, []string{labelRoute, labelStreaming}),
		Overhead: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "maskwire_overhead_seconds",
			Help:    "Time maskwire spent masking and restoring a request, excluding time waiting on the upstream.",
			Buckets: durationBuckets(), NativeHistogramBucketFactor: 1.1,
		}, []string{labelRoute, labelStreaming}),
		Reloads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_tenant_reloads_total", Help: "Tenant reloads triggered by SIGHUP, by result.",
		}, []string{"result"}),
		Holdback: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "maskwire_stream_holdback_bytes", Help: "Maximum bytes held back per streamed response.",
			Buckets: []float64{0, 1, 2, 4, 8, 16, 24, 32},
		}),
		ResponseDetections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_response_detections_total",
			Help: "Entities found in upstream responses that were not issued as tokens, by tenant and type.",
		}, []string{labelTenant, labelType}),
		StreamEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "maskwire_stream_events_total", Help: "Upstream SSE events by outcome.",
		}, []string{"outcome"}),
	}
	reg.MustRegister(m.Requests, m.Detections, m.Blocked, m.ForeignTokens, m.Restores, m.ToolWithheld, m.MaskDuration,
		m.RequestLatency, m.Overhead, m.Reloads, m.Holdback, m.StreamEvents, m.ResponseDetections)
	return m
}

func (m *Metrics) RegisterAudit(written, dropped func() float64) {
	m.Registry.MustRegister(
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{Name: "maskwire_audit_events_total", Help: "Audit records written."},
			written,
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{Name: "maskwire_audit_dropped_total", Help: "Audit records dropped."},
			dropped,
		),
	)
}
