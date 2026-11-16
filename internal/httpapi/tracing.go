package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
)

func requestAttrs(r *http.Request, rt *Route) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String("http.request.method", r.Method),
		attribute.String("http.route", rt.Path),
		attribute.String("maskwire.route", rt.Name),
	)
}

func annotate(span trace.Span, x *exchange) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.Int("http.response.status_code", x.status),
		attribute.Bool("maskwire.streaming", x.stream),
		attribute.Int64("maskwire.restored", int64(x.stats.Restored)),
		attribute.Int64("maskwire.foreign_tokens", int64(x.stats.Foreign)),
		attribute.Int64("maskwire.tool_withheld", int64(x.stats.Withheld)),
	)
	if x.tenant != nil {
		span.SetAttributes(attribute.String("maskwire.tenant", x.tenant.Mask.ID))
	}
	if x.status >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, strconv.Itoa(x.status))
	}
}

func (s *Server) maskSpan(ctx context.Context, x *exchange) {
	if !trace.SpanFromContext(ctx).IsRecording() {
		return
	}
	_, span := s.opts.Tracer.Start(ctx, "mask", trace.WithTimestamp(x.start))
	defer span.End(trace.WithTimestamp(x.start.Add(x.maskTime)))
	c := x.session.Counters()
	var types []string
	for t, n := range c.Detected {
		if n > 0 {
			types = append(types, entity.Type(t).String())
		}
	}
	span.SetAttributes(
		attribute.StringSlice("maskwire.entity_types", types),
		attribute.Int64("maskwire.detected", int64(c.TotalDetected())),
		attribute.Int64("maskwire.masked", int64(c.TotalAction(policy.Mask))),
		attribute.Int64("maskwire.redacted", int64(c.TotalAction(policy.Redact))),
		attribute.Int64("maskwire.blocked", int64(c.TotalAction(policy.Block))),
	)
}

func (s *Server) upstreamTransport(rt *Route) http.RoundTripper {
	if !s.tracing {
		return s.transport
	}
	return &tracingTransport{base: s.transport, tracer: s.opts.Tracer, prop: s.opts.Propagator, route: rt.Name}
}

type tracingTransport struct {
	base   http.RoundTripper
	tracer trace.Tracer
	prop   propagation.TextMapPropagator
	route  string
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := t.tracer.Start(req.Context(), "upstream "+t.route, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("server.address", req.URL.Host)))
	defer span.End()
	if !span.SpanContext().IsValid() {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("upstream round trip: %w", err)
		}
		return resp, nil
	}
	out := req.Clone(ctx)
	t.prop.Inject(ctx, propagation.HeaderCarrier(out.Header))
	resp, err := t.base.RoundTrip(out)
	if err != nil {
		span.SetStatus(codes.Error, "upstream error")
		return nil, fmt.Errorf("upstream round trip: %w", err)
	}
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	return resp, nil
}
