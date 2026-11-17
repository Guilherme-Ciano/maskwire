package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/httpapi"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/telemetry"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
	"github.com/Guilherme-Ciano/maskwire/internal/vault"
)

var (
	ErrUnknownCodec        = errors.New("unknown codec")
	ErrKeyVersion          = errors.New("key version out of range")
	ErrReloadDefaultTenant = errors.New("server.default_tenant cannot change on reload")
)

type App struct {
	closeVault func() error
	tracing    *telemetry.Tracing
	cfg        config.Config
	keyring    *token.Keyring
	log        *slog.Logger
	metrics    *telemetry.Metrics
	api        *httpapi.Server
	auditor    *audit.Writer
}

func New(cfg config.Config, log *slog.Logger) (*App, error) {
	master, err := cfg.MasterKey()
	if err != nil {
		return nil, fmt.Errorf("load master key: %w", err)
	}
	if cfg.Keys.Version < 1 || cfg.Keys.Version > math.MaxUint16 {
		return nil, ErrKeyVersion
	}
	kr, err := token.NewKeyring(master, uint16(cfg.Keys.Version))
	clear(master)
	if err != nil {
		return nil, fmt.Errorf("build keyring: %w", err)
	}
	tenants, err := buildTenants(cfg, kr)
	if err != nil {
		return nil, err
	}
	routes, err := buildRoutes(cfg)
	if err != nil {
		return nil, err
	}
	det := detect.New(detect.Options{
		Regions:           cfg.Detect.Regions,
		PhoneBudget:       cfg.Detect.PhoneBudget,
		DisablePhone:      cfg.Detect.DisablePhone,
		KeepReservedEmail: cfg.Detect.KeepReservedEmail,
	})
	metrics := telemetry.NewMetrics()
	tracing, err := telemetry.NewTracing(context.Background(), telemetry.TracingOptions{
		Endpoint: cfg.Tracing.Endpoint, SampleRatio: cfg.Tracing.SampleRatio, Version: buildVersion(),
	})
	if err != nil {
		return nil, err
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn("tracing export failed", slog.String("error", err.Error()))
	}))
	a := &App{cfg: cfg, log: log, metrics: metrics, tracing: tracing}
	v, closeVault, err := buildVault(cfg.Vault)
	if err != nil {
		return nil, err
	}
	a.closeVault = closeVault
	var auditor httpapi.Auditor
	if cfg.Audit.Enabled {
		onFull := audit.Block
		if cfg.Audit.OnFull == "drop" {
			onFull = audit.Drop
		}
		auditKey, err := kr.AuditKey()
		if err != nil {
			return nil, err
		}
		w, err := audit.NewWriter(
			audit.Options{
				Dir: cfg.Audit.Dir, Key: auditKey, Buffer: cfg.Audit.Buffer, OnFull: onFull,
				AnchorPath: cfg.Audit.AnchorFile,
			},
			log,
		)
		clear(auditKey)
		if err != nil {
			return nil, fmt.Errorf("open audit log: %w", err)
		}
		a.auditor = w
		auditor = w
		metrics.RegisterAudit(
			func() float64 { return float64(w.Written()) },
			func() float64 { return float64(w.Dropped()) },
		)
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   256,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: cfg.Server.UpstreamTimeout,
		DisableCompression:    false,
	}
	warnTenants(cfg, log)
	cfg.Keys.MasterKey = ""
	a.cfg = cfg
	a.keyring = kr
	a.api = httpapi.NewServer(
		log,
		metrics,
		mask.NewEngine(det, cfg.Detect.MaxEntities),
		tenants,
		routes,
		transport,
		auditor,
		httpapi.Options{
			TenantHeader:      cfg.Server.TenantHeader,
			Tracer:            tracing.Enabled(),
			Vault:             v,
			Propagator:        tracing.Propagator,
			SessionHeader:     cfg.Server.SessionHeader,
			DefaultTenant:     cfg.Server.DefaultTenant,
			MaxBodyBytes:      cfg.Server.MaxBodyBytes,
			StreamIdleTimeout: cfg.Server.StreamIdleTimeout,
		},
	)
	return a, nil
}

func warnTenants(cfg config.Config, log *slog.Logger) {
	for i := range cfg.Tenants {
		t := &cfg.Tenants[i]
		if t.Mode == "observe" {
			log.Warn("tenant in observe mode forwards raw personal data upstream", slog.String("tenant", t.ID))
		}
	}
	if cfg.Server.DefaultTenant != "" {
		log.Warn("default tenant accepts unauthenticated requests", slog.String("tenant", cfg.Server.DefaultTenant))
	}
}

func (a *App) ReloadTenants(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		a.metrics.Reloads.WithLabelValues("error").Inc()
		return fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.Server.DefaultTenant != a.cfg.Server.DefaultTenant {
		a.metrics.Reloads.WithLabelValues("error").Inc()
		return ErrReloadDefaultTenant
	}
	tenants, err := buildTenants(cfg, a.keyring)
	if err != nil {
		a.metrics.Reloads.WithLabelValues("error").Inc()
		return err
	}
	a.api.SetTenants(tenants)
	a.metrics.Reloads.WithLabelValues("ok").Inc()
	warnTenants(cfg, a.log)
	a.log.Info("tenants reloaded", slog.Int("tenants", len(tenants)))
	return nil
}

func scope(s string) httpapi.Scope {
	switch s {
	case "session":
		return httpapi.ScopeSession
	case "request":
		return httpapi.ScopeRequest
	}
	return httpapi.ScopeTenant
}

func buildTenants(cfg config.Config, kr *token.Keyring) ([]*httpapi.Tenant, error) {
	out := make([]*httpapi.Tenant, 0, len(cfg.Tenants))
	for i := range cfg.Tenants {
		t := &cfg.Tenants[i]
		p, err := t.Policy()
		if err != nil {
			return nil, fmt.Errorf("tenant %s policy: %w", t.ID, err)
		}
		key, err := kr.Tenant(t.ID)
		if err != nil {
			return nil, fmt.Errorf("tenant %s key: %w", t.ID, err)
		}
		var h [32]byte
		if _, err := hex.Decode(h[:], []byte(t.KeySHA256)); err != nil {
			return nil, fmt.Errorf("tenant %s key hash: %w", t.ID, err)
		}
		withhold, err := t.ToolWithhold()
		if err != nil {
			return nil, fmt.Errorf("tenant %s: %w", t.ID, err)
		}
		foreign := restore.ForeignGeneric
		if t.Foreign == "keep" {
			foreign = restore.ForeignKeep
		}
		out = append(out, &httpapi.Tenant{
			Mask:         &mask.Tenant{ID: t.ID, Key: key, Policy: p, TokenLength: t.TokenLength},
			KeyHash:      h,
			Foreign:      foreign,
			ToolWithhold: withhold,
			Limit:        httpapi.NewLimiter(t.RateLimit.RPS, t.RateLimit.Burst),
			Scope:        scope(t.TokenScope),
			CrossTurn:    t.CrossTurn,
			PromptHint:   t.PromptHint,
			ResponseScan: t.ResponseScan != "off",
		})
	}
	return out, nil
}

func buildRoutes(cfg config.Config) ([]httpapi.Route, error) {
	out := make([]httpapi.Route, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		u, err := url.Parse(r.Upstream)
		if err != nil {
			return nil, fmt.Errorf("route %s upstream: %w", r.Name, err)
		}
		var c httpapi.Codec
		switch r.Codec {
		case "openai":
			c = httpapi.NewOpenAICodec()
		case "anthropic":
			c = httpapi.NewAnthropicCodec()
		case "openai-responses":
			c = httpapi.NewResponsesCodec()
		case "openai-embeddings":
			c = httpapi.NewEmbeddingsCodec()
		case "passthrough":
			c = nil
		default:
			return nil, fmt.Errorf("route %s: %w %q", r.Name, ErrUnknownCodec, r.Codec)
		}
		out = append(
			out,
			httpapi.Route{Name: r.Name, Path: r.Path, Upstream: u, Codec: c, Synthesize: r.SynthesizeStream},
		)
	}
	return out, nil
}

func (a *App) Handler() http.Handler {
	return a.api.Handler()
}

func (a *App) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(a.metrics.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeText(w, "ok\n")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !a.api.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		writeText(w, "ready\n")
	})
	if a.cfg.Server.Pprof {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}
	return mux
}

func writeText(w http.ResponseWriter, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		return
	}
}

func (a *App) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              a.cfg.Server.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: a.cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       a.cfg.Server.ReadTimeout,
		IdleTimeout:       a.cfg.Server.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	admin := &http.Server{
		Addr:              a.cfg.Server.AdminAddr,
		Handler:           a.AdminHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	auditCtx, stopAudit := context.WithCancel(context.WithoutCancel(ctx))
	defer stopAudit()
	g, gctx := errgroup.WithContext(ctx)
	auditDone := make(chan error, 1)
	if a.auditor != nil {
		go func() { auditDone <- a.auditor.Run(auditCtx) }()
	} else {
		auditDone <- nil
	}
	for _, s := range []*http.Server{srv, admin} {
		g.Go(func() error {
			a.log.InfoContext(ctx, "listening", slog.String("addr", s.Addr))
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("listen %s: %w", s.Addr, err)
			}
			return nil
		})
	}
	g.Go(func() error {
		<-gctx.Done()
		a.api.SetReady(false)
		if ctx.Err() != nil && a.cfg.Server.ShutdownDrainDelay > 0 {
			time.Sleep(a.cfg.Server.ShutdownDrainDelay)
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.cfg.Server.ShutdownTimeout)
		defer cancel()
		return errors.Join(shutdown(sctx, srv), shutdown(sctx, admin))
	})
	err := g.Wait()
	stopAudit()
	tctx, tcancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer tcancel()
	return errors.Join(err, <-auditDone, a.tracing.Shutdown(tctx), a.closeVault())
}

func buildVault(c config.Vault) (*vault.Vault, func() error, error) {
	switch c.Backend {
	case "memory":
		return vault.New(vault.NewMemory(c.MaxEntries), c.TTL), func() error { return nil }, nil
	case "redis":
		password := ""
		if c.RedisPasswordFile != "" {
			b, err := os.ReadFile(filepath.Clean(c.RedisPasswordFile))
			if err != nil {
				return nil, nil, fmt.Errorf("read vault redis password: %w", err)
			}
			password = strings.TrimSpace(string(b))
		}
		r := vault.NewRedis(c.RedisAddr, password)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.Ping(ctx); err != nil {
			return nil, nil, errors.Join(err, r.Close())
		}
		return vault.New(r, c.TTL), r.Close, nil
	}
	return nil, func() error { return nil }, nil
}

func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

func shutdown(ctx context.Context, s *http.Server) error {
	if err := s.Shutdown(ctx); err != nil {
		return errors.Join(fmt.Errorf("shutdown %s: %w", s.Addr, err), s.Close())
	}
	return nil
}
