package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/openai"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/telemetry"
)

const (
	maxResponseBytes = 32 << 20
	HeaderMasked     = "X-Maskwire-Masked"
	HeaderRestored   = "X-Maskwire-Restored"

	statusClientClosed = 499
)

var (
	ErrUnauthorized     = errors.New("missing or invalid tenant key")
	ErrUnsupportedMedia = errors.New("compressed request bodies are not supported")
	ErrUpstreamEncoding = errors.New("upstream returned an encoded body")
	ErrResponseTooLarge = errors.New("upstream response too large")
	ErrAuditUnavailable = errors.New("audit log unavailable")
)

type Tenant struct {
	Mask    *mask.Tenant
	KeyHash [32]byte
	Foreign restore.Foreign
}

type Route struct {
	Name       string
	Path       string
	Upstream   *url.URL
	Codec      Codec
	Synthesize bool
	proxy      *httputil.ReverseProxy
}

type Auditor interface {
	Enqueue(ctx context.Context, e *audit.Event) error
	Healthy() bool
}

type Options struct {
	TenantHeader      string
	DefaultTenant     string
	MaxBodyBytes      int64
	StreamIdleTimeout time.Duration
}

type Server struct {
	log       *slog.Logger
	metrics   *telemetry.Metrics
	engine    *mask.Engine
	tenants   []*Tenant
	byID      map[string]*Tenant
	routes    []Route
	transport http.RoundTripper
	auditor   Auditor
	opts      Options
	ready     atomic.Bool
}

func NewServer(log *slog.Logger, m *telemetry.Metrics, eng *mask.Engine, tenants []*Tenant, routes []Route,
	transport http.RoundTripper, auditor Auditor, o Options,
) *Server {
	s := &Server{
		log: log, metrics: m, engine: eng, tenants: tenants, routes: routes,
		transport: transport, auditor: auditor, opts: o, byID: map[string]*Tenant{},
	}
	for _, t := range tenants {
		s.byID[t.Mask.ID] = t
	}
	for i := range s.routes {
		if s.routes[i].Codec != nil {
			s.routes[i].proxy = s.routeProxy(&s.routes[i])
		}
	}
	s.ready.Store(true)
	return s
}

func (s *Server) SetReady(v bool) {
	s.ready.Store(v)
}

func (s *Server) Ready() bool {
	return s.ready.Load() && (s.auditor == nil || s.auditor.Healthy())
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for i := range s.routes {
		rt := &s.routes[i]
		if rt.Codec == nil {
			h := s.recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s.passthrough(w, r, rt)
			}))
			mux.Handle("GET "+rt.Path, h)
			if !strings.HasSuffix(rt.Path, "/") {
				mux.Handle("GET "+rt.Path+"/", h)
			}
			continue
		}
		mux.Handle("POST "+rt.Path, s.recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.proxy(w, r, rt)
		})))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not configured")
	})
	return mux
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer s.handlePanic(sw, r)
		next.ServeHTTP(sw, r)
	})
}

func (s *Server) handlePanic(w *statusWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
	if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(v)
	}
	if w.wrote {
		s.log.ErrorContext(
			r.Context(),
			"handler panic after response started",
			slog.String("panic_type", fmt.Sprintf("%T", v)),
		)
		panic(http.ErrAbortHandler)
	}
	s.log.ErrorContext(r.Context(), "handler panic", slog.String("panic_type", fmt.Sprintf("%T", v)))
	writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
}

func (s *Server) passthrough(w http.ResponseWriter, r *http.Request, rt *Route) {
	tenant, err := s.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "passthrough routes do not accept a request body")
		return
	}
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	rp := &httputil.ReverseProxy{
		Transport: s.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(rt.Upstream)
			pr.Out.Header.Del(s.opts.TenantHeader)
			pr.Out.Body = http.NoBody
			pr.Out.ContentLength = 0
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			s.upstreamError(w, req, &exchange{route: rt}, err)
		},
	}
	defer func() {
		s.metrics.Requests.WithLabelValues(tenant.Mask.ID, rt.Name, strconv.Itoa(sw.status)).Inc()
	}()
	rp.ServeHTTP(sw, r)
}

func (s *Server) authenticate(r *http.Request) (*Tenant, error) {
	key := r.Header.Get(s.opts.TenantHeader)
	if key == "" {
		if t, ok := s.byID[s.opts.DefaultTenant]; ok {
			return t, nil
		}
		return nil, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(key))
	var found *Tenant
	for _, t := range s.tenants {
		if subtle.ConstantTimeCompare(sum[:], t.KeyHash[:]) == 1 {
			found = t
		}
	}
	if found == nil {
		return nil, ErrUnauthorized
	}
	return found, nil
}

type exchange struct {
	start    time.Time
	tenant   *Tenant
	route    *Route
	meta     Meta
	session  *mask.Session
	maskTime time.Duration
	inDigest [32]byte
	outHash  hash.Hash
	stats    restore.Stats
	stream   bool
	stalled  bool
	observe  bool
	masked   []byte
	onStall  func()
	status   int
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, rt *Route) {
	x := &exchange{start: time.Now(), route: rt, status: http.StatusOK}
	tenant, err := s.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	x.tenant = tenant
	observe := tenant.Mask.Policy.Mode == policy.Observe
	if s.auditor != nil && !s.auditor.Healthy() {
		s.fail(w, r, x, http.StatusServiceUnavailable, "audit_unavailable", ErrAuditUnavailable)
		return
	}
	if ce := r.Header.Get("Content-Encoding"); ce != "" && ce != "identity" {
		s.fail(w, r, x, http.StatusUnsupportedMediaType, "unsupported_encoding", ErrUnsupportedMedia)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.opts.MaxBodyBytes))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			s.fail(w, r, x, http.StatusRequestEntityTooLarge, "body_too_large", errors.New("request body too large"))
			return
		}
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", errors.New("could not read request body"))
		return
	}
	x.meta, err = rt.Codec.ParseMeta(body)
	if err != nil {
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", errors.New("request body is not valid JSON"))
		return
	}
	if x.meta.Logprobs {
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", openai.ErrLogprobs)
		return
	}
	x.session = s.engine.NewSession(tenant.Mask)
	defer x.session.Release()
	masked, err := rt.Codec.MaskRequest(make([]byte, 0, len(body)+len(body)/4), body, x.session)
	x.maskTime = time.Since(x.start)
	s.metrics.MaskDuration.WithLabelValues(rt.Name).Observe(x.maskTime.Seconds())
	s.countDetections(x)
	switch {
	case errors.Is(err, mask.ErrTooManyEntities):
		s.fail(w, r, x, http.StatusUnprocessableEntity, "too_many_entities", err)
		return
	case err != nil:
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", errors.New("request body could not be processed"))
		return
	}
	if blocked, ok := errors.AsType[*mask.BlockedError](x.session.Err()); ok {
		for _, t := range blocked.Types {
			s.metrics.Blocked.WithLabelValues(tenant.Mask.ID, t.String()).Inc()
		}
		x.inDigest = sha256.Sum256(masked)
		s.failBlocked(w, r, x, blocked)
		return
	}
	if observe {
		masked = body
	}
	x.inDigest = sha256.Sum256(masked)
	s.forward(w, r, x, masked, observe)
}

type exchangeKey struct{}

func exchangeFrom(ctx context.Context) *exchange {
	x, ok := ctx.Value(exchangeKey{}).(*exchange)
	if !ok {
		return nil
	}
	return x
}

func (s *Server) routeProxy(rt *Route) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: s.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			x := exchangeFrom(pr.In.Context())
			pr.SetURL(rt.Upstream)
			if p := rt.Upstream.Path; p != "" && p != "/" {
				pr.Out.URL.Path, pr.Out.URL.RawPath = p, ""
			}
			pr.Out.Header.Del(s.opts.TenantHeader)
			pr.Out.Header.Del("Content-Length")
			pr.Out.Header.Del("Accept-Encoding")
			pr.Out.Body = io.NopCloser(bytes.NewReader(x.masked))
			pr.Out.ContentLength = int64(len(x.masked))
			pr.Out.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(x.masked)), nil
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			return s.modifyResponse(resp, exchangeFrom(resp.Request.Context()))
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			s.upstreamError(w, req, exchangeFrom(req.Context()), err)
		},
	}
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, x *exchange, masked []byte, observe bool) {
	ctx, cancel := context.WithCancelCause(r.Context())
	defer cancel(nil)
	x.masked, x.observe = masked, observe
	x.onStall = func() {
		x.stalled = true
		cancel(codec.ErrUpstreamStalled)
	}
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		x.status = sw.status
		if v := recover(); v != nil {
			x.status = statusClientClosed
			if x.stalled {
				x.status = http.StatusGatewayTimeout
			}
			s.finish(r, x)
			panic(v)
		}
		s.finish(r, x)
	}()
	x.route.proxy.ServeHTTP(sw, r.WithContext(context.WithValue(ctx, exchangeKey{}, x)))
}

func (s *Server) modifyResponse(resp *http.Response, x *exchange) error {
	observe, onStall := x.observe, x.onStall
	masked := uint64(x.session.Counters().TotalAction(policy.Mask))
	if observe {
		masked = 0
	}
	resp.Header.Set(HeaderMasked, strconv.FormatUint(masked, 10))
	if observe || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "" && ce != "identity" {
		return ErrUpstreamEncoding
	}
	x.outHash = sha256.New()
	ct, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		ct = ""
	}
	opts := restore.Options{Foreign: x.tenant.Foreign}
	if ct == "text/event-stream" {
		x.stream = true
		src := teeCloser{Reader: io.TeeReader(resp.Body, x.outHash), Closer: resp.Body}
		resp.Body = x.route.Codec.NewStreamBody(src, x.session, StreamOptions{
			Restore: opts,
			Idle:    s.opts.StreamIdleTimeout,
			OnStall: onStall,
			OnClose: func(st restore.Stats, ev, dropped int) {
				x.stats = st
				s.metrics.StreamEvents.WithLabelValues("forwarded").Add(float64(ev - dropped))
				s.metrics.StreamEvents.WithLabelValues("dropped").Add(float64(dropped))
			},
		})
		resp.ContentLength = -1
		resp.Header.Del("Content-Length")
		resp.Header.Del(HeaderRestored)
		resp.Header.Set("X-Accel-Buffering", "no")
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close upstream body: %w", closeErr)
	}
	if len(raw) > maxResponseBytes {
		return ErrResponseTooLarge
	}
	x.outHash.Write(raw)
	out, st, err := x.route.Codec.RestoreBody(make([]byte, 0, len(raw)+256), raw, x.session, opts)
	if err != nil {
		return fmt.Errorf("restore body: %w", err)
	}
	x.stats = st
	resp.Header.Set(HeaderRestored, strconv.FormatUint(uint64(st.Restored), 10))
	if x.meta.Stream && x.route.Synthesize {
		synth, err := x.route.Codec.Synthesize(nil, out)
		if err != nil {
			return fmt.Errorf("synthesize stream: %w", err)
		}
		out = synth
		x.stream = true
		resp.Header.Set("Content-Type", "text/event-stream")
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return nil
}

func (s *Server) upstreamError(w http.ResponseWriter, r *http.Request, x *exchange, err error) {
	status, kind := http.StatusBadGateway, "upstream_error"
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		status, kind = http.StatusGatewayTimeout, "upstream_timeout"
	case errors.Is(err, context.Canceled):
		status, kind = statusClientClosed, "client_closed"
	case errors.Is(err, ErrUpstreamEncoding), errors.Is(err, ErrResponseTooLarge):
		kind = "upstream_unsupported"
	}
	s.log.WarnContext(r.Context(), "upstream request failed",
		slog.String("route", x.route.Name), slog.String("kind", kind), slog.Int("status", status))
	writeError(w, status, kind, "upstream request failed")
}

func (s *Server) countDetections(x *exchange) {
	c := x.session.Counters()
	for t := range c.Actions {
		for a, n := range c.Actions[t] {
			if n > 0 {
				s.metrics.Detections.WithLabelValues(x.tenant.Mask.ID, entity.Type(t).String(), policy.Action(a).String()).
					Add(float64(n))
			}
		}
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, x *exchange, status int, kind string, err error) {
	writeError(w, status, kind, err.Error())
	x.status = status
	if x.tenant != nil {
		s.finish(r, x)
	}
}

func (s *Server) failBlocked(w http.ResponseWriter, r *http.Request, x *exchange, b *mask.BlockedError) {
	names := make([]string, len(b.Types))
	for i, t := range b.Types {
		names[i] = t.String()
	}
	writeBlocked(w, names)
	x.status = http.StatusUnprocessableEntity
	s.finish(r, x)
}

func (s *Server) finish(r *http.Request, x *exchange) {
	total := time.Since(x.start)
	tenantID := x.tenant.Mask.ID
	s.metrics.Requests.WithLabelValues(tenantID, x.route.Name, strconv.Itoa(x.status)).Inc()
	s.metrics.RequestLatency.WithLabelValues(x.route.Name, strconv.FormatBool(x.stream)).Observe(total.Seconds())
	if x.stats.Restored > 0 {
		s.metrics.Restores.WithLabelValues(tenantID, strconv.FormatBool(x.stream)).Add(float64(x.stats.Restored))
	}
	for t, n := range x.stats.ForeignTypes {
		if n > 0 {
			s.metrics.ForeignTokens.WithLabelValues(tenantID, entity.Type(t).String()).Add(float64(n))
		}
	}
	if x.stream {
		s.metrics.Holdback.Observe(float64(x.stats.MaxHeld))
	}
	if s.auditor == nil {
		return
	}
	ev := audit.Event{
		TS: x.start, Tenant: tenantID, Route: x.route.Name, Upstream: x.route.Upstream.Host, Model: x.meta.Model,
		Mode: x.tenant.Mask.Policy.Mode.String(), Status: x.status, Streaming: x.stream,
		Restored: x.stats.Restored, Foreign: x.stats.Foreign, InputDigest: x.inDigest,
		MaskMicros: x.maskTime.Microseconds(), TotalMicros: total.Microseconds(),
	}
	if x.session != nil {
		c := x.session.Counters()
		ev.Detected = c.Detected
		ev.Mask, ev.Redact = c.TotalAction(policy.Mask), c.TotalAction(policy.Redact)
		ev.Block, ev.Allow = c.TotalAction(policy.Block), c.TotalAction(policy.Allow)
	}
	if x.outHash != nil {
		ev.HasOutput = true
		x.outHash.Sum(ev.OutputDigest[:0])
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.auditor.Enqueue(ctx, &ev); err != nil {
		s.log.ErrorContext(r.Context(), "audit enqueue failed", slog.String("error", err.Error()))
	}
}

type teeCloser struct {
	io.Reader
	io.Closer
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if code >= http.StatusOK && !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func DigestHex(d [32]byte) string {
	return hex.EncodeToString(d[:])
}
