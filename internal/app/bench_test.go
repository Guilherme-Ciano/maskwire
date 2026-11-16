package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/mockllm"
)

func BenchmarkProxyRequest(b *testing.B) {
	mock := httptest.NewServer(mockllm.New(mockllm.Options{}).Handler())
	defer mock.Close()
	cfg := config.Defaults()
	cfg.Keys.MasterKey = strings.Repeat("ab", 32)
	cfg.Audit.Enabled = false
	cfg.Routes = []config.Route{{Name: "openai", Path: chatPath, Upstream: mock.URL, Codec: "openai"}}
	cfg.Tenants = []config.Tenant{{ID: "acme", KeySHA256: config.HashKey(tenantKey)}}
	a, err := New(cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		b.Fatal(err)
	}
	h := a.Handler()
	body := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat(
		"Reply to ana.souza@correio.pt about CPF 529.982.247-25. ",
		40,
	) + `"}]}`
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequestWithContext(b.Context(), http.MethodPost, chatPath, strings.NewReader(body))
		req.Header.Set(keyHeader, tenantKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
		if _, err := io.Copy(io.Discard, rec.Body); err != nil {
			b.Fatal(err)
		}
	}
}
