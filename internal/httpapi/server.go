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

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/bufpool"
	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/openai"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/telemetry"
	"github.com/Guilherme-Ciano/maskwire/internal/vault"
)

const (
	maxResponseBytes = 32 << 20
	HeaderMasked     = "X-Maskwire-Masked"
	HeaderRestored   = "X-Maskwire-Restored"

	statusClientClosed = 499
	minTenantKey       = 16
)

var (
	ErrUnauthorized     = errors.New("missing or invalid tenant key")
	ErrUnsupportedMedia = errors.New("compressed request bodies are not supported")
	ErrRateLimited      = errors.New("tenant request rate exceeded")
	ErrUpstreamEncoding = errors.New("upstream returned an encoded body")
	ErrResponseTooLarge = errors.New("upstream response too large")
	ErrAuditUnavailable = errors.New("audit log unavailable")
)

type Tenant struct {
	Mask         *mask.Tenant
	KeyHash      [32]byte
	Foreign      restore.Foreign
	ToolWithhold entity.Mask
	PromptHint   bool
	ResponseScan bool
	Limit        *Limiter
	Scope        Scope
	CrossTurn    bool
}

type Scope uint8

const (
	ScopeTenant Scope = iota
	ScopeSession
	ScopeRequest
)

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
	SessionHeader     string
	Vault             *vault.Vault
	Tracer            trace.Tracer
	Propagator        propagation.TextMapPropagator
	DefaultTenant     string
	MaxBodyBytes      int64
	StreamIdleTimeout time.Duration
}

type Server struct {
	log       *slog.Logger
	metrics   *telemetry.Metrics
	engine    *mask.Engine
	tenants   atomic.Pointer[tenantTable]
	routes    []Route
	transport http.RoundTripper
	bufs      bufpool.Pool
	auditor   Auditor
	opts      Options
	ready     atomic.Bool
	tracing   bool
}

func NewServer(log *slog.Logger, m *telemetry.Metrics, eng *mask.Engine, tenants []*Tenant, routes []Route,
	transport http.RoundTripper, auditor Auditor, o Options,
) *Server {
	s := &Server{
		log: log, metrics: m, engine: eng, routes: routes,
		transport: transport, auditor: auditor, opts: o,
	}
	s.tracing = s.opts.Tracer != nil
	if s.opts.Propagator == nil {
		s.opts.Propagator = propagation.TraceContext{}
	}
	s.SetTenants(tenants)
	for i := range s.routes {
		if s.routes[i].Codec != nil {
			s.routes[i].proxy = s.routeProxy(&s.routes[i])
		}
	}
	s.ready.Store(true)
	return s
}

type tenantTable struct {
	list []*Tenant
	byID map[string]*Tenant
}

func (s *Server) SetTenants(tenants []*Tenant) {
	t := &tenantTable{list: tenants, byID: make(map[string]*Tenant, len(tenants))}
	old := s.tenants.Load()
	for _, n := range tenants {
		t.byID[n.Mask.ID] = n
		if old == nil {
			continue
		}
		if prev, ok := old.byID[n.Mask.ID]; ok && prev.Limit.Same(n.Limit) {
			n.Limit = prev.Limit
		}
	}
	s.tenants.Store(t)
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
	if !s.admit(w, tenant, rt) {
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
			pr.Out.Header.Del(s.opts.SessionHeader)
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
	table := s.tenants.Load()
	key := r.Header.Get(s.opts.TenantHeader)
	if key == "" {
		if t, ok := table.byID[s.opts.DefaultTenant]; ok {
			return t, nil
		}
		return nil, ErrUnauthorized
	}
	if len(key) < minTenantKey {
		return nil, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(key))
	var found *Tenant
	for _, t := range table.list {
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
	vault    *vault.Scope
	maskTime time.Duration
	restTime time.Duration
	waitTime time.Duration
	inDigest [32]byte
	outHash  hash.Hash
	stats    restore.Stats
	stream   bool
	stalled  bool
	observe  bool
	payload  *bufpool.Buf
	upstream []byte
	onStall  func()
	status   int
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, rt *Route, x *exchange) (*Tenant, bool) {
	tenant, err := s.authenticate(r)
	if err != nil {
		x.status = http.StatusUnauthorized
		writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return nil, false
	}
	if !s.admit(w, tenant, rt) {
		x.status = http.StatusTooManyRequests
		return nil, false
	}
	x.tenant = tenant
	return tenant, true
}

func (s *Server) readBody(w http.ResponseWriter, r *http.Request) (*bufpool.Buf, error) {
	src := http.MaxBytesReader(w, r.Body, s.opts.MaxBodyBytes)
	if n := r.ContentLength; n > 0 && n <= s.opts.MaxBodyBytes {
		buf := s.bufs.Get(int(n))
		buf.B = buf.B[:n]
		if _, err := io.ReadFull(src, buf.B); err != nil {
			buf.Release()
			return nil, fmt.Errorf("read request body: %w", err)
		}
		return buf, nil
	}
	body, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	return bufpool.Wrap(body), nil
}

func (s *Server) readRequest(w http.ResponseWriter, r *http.Request, rt *Route, x *exchange) (*bufpool.Buf, bool) {
	buf, err := s.readBody(w, r)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			s.fail(w, r, x, http.StatusRequestEntityTooLarge, "body_too_large", errors.New("request body too large"))
			return nil, false
		}
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", errors.New("could not read request body"))
		return nil, false
	}
	x.meta, err = rt.Codec.ParseMeta(buf.B)
	if err != nil {
		buf.Release()
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", errors.New("request body is not valid JSON"))
		return nil, false
	}
	if x.meta.Logprobs {
		buf.Release()
		s.fail(w, r, x, http.StatusBadRequest, "invalid_request", openai.ErrLogprobs)
		return nil, false
	}
	return buf, true
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, rt *Route) {
	x := &exchange{start: time.Now(), route: rt, status: http.StatusOK}
	if s.tracing {
		ctx := s.opts.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := s.opts.Tracer.Start(
			ctx,
			"maskwire "+rt.Name,
			trace.WithSpanKind(trace.SpanKindServer),
			requestAttrs(r, rt),
		)
		defer span.End()
		defer annotate(span, x)
		r = r.WithContext(ctx)
	}
	tenant, ok := s.authorize(w, r, rt, x)
	if !ok {
		return
	}
	observe := tenant.Mask.Policy.Mode == policy.Observe
	if s.auditor != nil && !s.auditor.Healthy() {
		s.fail(w, r, x, http.StatusServiceUnavailable, "audit_unavailable", ErrAuditUnavailable)
		return
	}
	if ce := r.Header.Get("Content-Encoding"); ce != "" && ce != "identity" {
		s.fail(w, r, x, http.StatusUnsupportedMediaType, "unsupported_encoding", ErrUnsupportedMedia)
		return
	}
	in, ok := s.readRequest(w, r, rt, x)
	if !ok {
		return
	}
	defer in.Release()
	body := in.B
	x.session, x.vault = s.newSession(r, tenant)
	defer x.session.Release()
	out := s.bufs.Get(len(body) + len(body)/4)
	defer out.Release()
	masked, err := rt.Codec.MaskRequest(out.B, body, x.session)
	out.B = masked
	x.maskTime = time.Since(x.start)
	s.metrics.MaskDuration.WithLabelValues(rt.Name).Observe(x.maskTime.Seconds())
	s.countDetections(x)
	if s.tracing {
		s.maskSpan(r.Context(), x)
	}
	switch {
	case errors.Is(err, mask.ErrTooManyEntities):
		s.fail(w, r, x, http.StatusUnprocessableEntity, "too_many_entities", mask.ErrTooManyEntities)
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
	payload := out
	switch {
	case observe:
		payload = in
	case tenant.PromptHint && x.session.Issued() > 0:
		if hinted, err := rt.Codec.InjectHint(masked); err == nil {
			payload = bufpool.Wrap(hinted)
		}
	}
	x.inDigest = sha256.Sum256(payload.B)
	if !observe {
		s.saveVault(r.Context(), x)
	}
	s.forward(w, r, x, payload, observe)
}

const maxScanned = 64 << 10

func (x *exchange) tap(b []byte) {
	if room := maxScanned - len(x.upstream); room > 0 {
		x.upstream = append(x.upstream, b[:min(len(b), room)]...)
	}
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
		Transport: s.upstreamTransport(rt),
		Rewrite: func(pr *httputil.ProxyRequest) {
			x := exchangeFrom(pr.In.Context())
			pr.SetURL(rt.Upstream)
			if p := rt.Upstream.Path; p != "" && p != "/" {
				pr.Out.URL.Path, pr.Out.URL.RawPath = p, ""
			}
			pr.Out.Header.Del(s.opts.TenantHeader)
			pr.Out.Header.Del(s.opts.SessionHeader)
			pr.Out.Header.Del("Content-Length")
			pr.Out.Header.Del("Accept-Encoding")
			pr.Out.Body = x.payload.Reader()
			pr.Out.ContentLength = int64(len(x.payload.B))
			pr.Out.GetBody = func() (io.ReadCloser, error) {
				return x.payload.Reader(), nil
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

func (s *Server) forward(w http.ResponseWriter, r *http.Request, x *exchange, payload *bufpool.Buf, observe bool) {
	ctx, cancel := context.WithCancelCause(r.Context())
	defer cancel(nil)
	x.payload, x.observe = payload, observe
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
	opts := restore.Options{Foreign: x.tenant.Foreign, Withhold: x.tenant.ToolWithhold}
	if x.tenant.ResponseScan {
		opts.Tap = x.tap
	}
	if ct == "text/event-stream" {
		x.stream = true
		src := teeCloser{Reader: &timedReader{r: io.TeeReader(resp.Body, x.outHash), d: &x.waitTime}, Closer: resp.Body}
		if resp.Trailer == nil {
			resp.Trailer = http.Header{}
		}
		resp.Trailer[HeaderRestored] = nil
		resp.Body = x.route.Codec.NewStreamBody(src, x.restoreSet(), StreamOptions{
			Restore: opts,
			Idle:    s.opts.StreamIdleTimeout,
			OnStall: onStall,
			OnClose: func(st restore.Stats, ev, dropped int) {
				x.stats = st
				resp.Trailer.Set(HeaderRestored, strconv.FormatUint(uint64(st.Restored), 10))
				s.metrics.StreamEvents.WithLabelValues("forwarded").Add(float64(ev - dropped))
				s.metrics.StreamEvents.WithLabelValues("dropped").Add(float64(dropped))
			},
		})
		resp.Body = &timedBody{ReadCloser: resp.Body, d: &x.restTime}
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
	began := time.Now()
	defer func() { x.restTime += time.Since(began) }()
	out, st, err := x.route.Codec.RestoreBody(make([]byte, 0, len(raw)+256), raw, x.restoreSet(), opts)
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
	overhead := x.maskTime + max(x.restTime-x.waitTime, 0)
	s.metrics.Overhead.WithLabelValues(x.route.Name, strconv.FormatBool(x.stream)).Observe(overhead.Seconds())
	if x.stats.Restored > 0 {
		s.metrics.Restores.WithLabelValues(tenantID, strconv.FormatBool(x.stream)).Add(float64(x.stats.Restored))
	}
	if x.stats.Withheld > 0 {
		s.metrics.ToolWithheld.WithLabelValues(tenantID).Add(float64(x.stats.Withheld))
	}
	for t, n := range x.stats.ForeignTypes {
		if n > 0 {
			s.metrics.ForeignTokens.WithLabelValues(tenantID, entity.Type(t).String()).Add(float64(n))
		}
	}
	if x.stream {
		s.metrics.Holdback.Observe(float64(x.stats.MaxHeld))
	}
	var generated [entity.Count]uint32
	if len(x.upstream) > 0 {
		generated = s.engine.Scan(x.upstream)
		for t, n := range generated {
			if n > 0 {
				s.metrics.ResponseDetections.WithLabelValues(tenantID, entity.Type(t).String()).Add(float64(n))
			}
		}
	}
	if s.auditor == nil {
		return
	}
	ev := audit.Event{
		Generated: generated,
		TS:        x.start, Tenant: tenantID, Route: x.route.Name, Upstream: x.route.Upstream.Host, Model: x.meta.Model,
		Mode: x.tenant.Mask.Policy.Mode.String(), Status: x.status, Streaming: x.stream,
		Restored: x.stats.Restored, Foreign: x.stats.Foreign, Withheld: x.stats.Withheld, InputDigest: x.inDigest,
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

type timedReader struct {
	r io.Reader
	d *time.Duration
}

func (t *timedReader) Read(p []byte) (int, error) {
	began := time.Now()
	n, err := t.r.Read(p)
	*t.d += time.Since(began)
	return n, readErr(err, "read upstream")
}

func readErr(err error, op string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, io.EOF):
		return io.EOF
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

type timedBody struct {
	io.ReadCloser
	d *time.Duration
}

func (t *timedBody) Read(p []byte) (int, error) {
	began := time.Now()
	n, err := t.ReadCloser.Read(p)
	*t.d += time.Since(began)
	return n, readErr(err, "read response")
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
