package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
)

const upstreamReply = `{"id":"x","object":"chat.completion","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

func TestUpstreamH2C(t *testing.T) {
	t.Parallel()
	for _, h2c := range []bool{false, true} {
		var proto atomic.Int32
		up := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proto.Store(int32(r.ProtoMajor))
			w.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(w, upstreamReply); err != nil {
				t.Error(err)
			}
		}))
		var p http.Protocols
		p.SetHTTP1(true)
		p.SetUnencryptedHTTP2(true)
		up.Config.Protocols = &p
		up.Start()
		cfg := config.Defaults()
		cfg.Keys.MasterKey = strings.Repeat("ab", 32)
		cfg.Audit.Enabled = false
		cfg.Server.UpstreamH2C = h2c
		cfg.Routes = []config.Route{{Name: "openai", Path: chatPath, Upstream: up.URL, Codec: "openai"}}
		cfg.Tenants = []config.Tenant{{ID: "acme", KeySHA256: config.HashKey(tenantKey)}}
		a, err := New(cfg, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, chatPath,
			strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set(keyHeader, tenantKey)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		up.Close()
		if rec.Code != http.StatusOK {
			t.Fatalf("h2c=%v status %d: %s", h2c, rec.Code, rec.Body)
		}
		want := int32(1)
		if h2c {
			want = 2
		}
		if got := proto.Load(); got != want {
			t.Fatalf("h2c=%v: upstream saw HTTP/%d, want HTTP/%d", h2c, got, want)
		}
	}
}
