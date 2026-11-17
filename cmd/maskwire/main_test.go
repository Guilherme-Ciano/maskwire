package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/app"
	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRoot()
	var out, errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestDetectCommand(t *testing.T) {
	out, counts, err := run(t, "CPF 529.982.247-25 and ana@empresa.pt", "detect")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "529.982") || strings.Contains(out, "ana@") || !strings.Contains(out, "[CPF_") {
		t.Fatalf("output %q", out)
	}
	if !strings.Contains(counts, "CPF\t1") || !strings.Contains(counts, "EMAIL\t1") {
		t.Fatalf("counts %q", counts)
	}
}

func TestHashKeyCommand(t *testing.T) {
	key := "mw-demo-0b8e3c1f5a7d9e2b"
	out, _, err := run(t, key+"\n", "hash-key")
	if err != nil || strings.TrimSpace(out) != config.HashKey(key) {
		t.Fatalf("%q %v", out, err)
	}
	for _, weak := range []string{"\n", "demo-key\n"} {
		if _, _, err := run(t, weak, "hash-key"); err == nil {
			t.Fatalf("%q must be refused", weak)
		}
	}
	gen, _, err := run(t, "", "gen-key")
	if err != nil || !strings.Contains(gen, "key:        mw_") || !strings.Contains(gen, "key_sha256: ") {
		t.Fatalf("gen-key: %q %v", gen, err)
	}
}

func TestAuditVerifyCommand(t *testing.T) {
	master := bytes.Repeat([]byte{5}, 32)
	t.Setenv("MASKWIRE_KEYS__MASTER_KEY", hex.EncodeToString(master))
	t.Setenv("MASKWIRE_CONFIG", "")
	kr, err := token.NewKeyring(master, 1)
	if err != nil {
		t.Fatal(err)
	}
	key, err := kr.AuditKey()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	w, err := audit.NewWriter(audit.Options{Dir: dir, Key: key}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	ev := audit.Event{TS: time.Now(), Tenant: "acme", Route: "chat", Status: 200}
	if err := w.Enqueue(ctx, &ev); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "", "audit", "verify", "--dir", dir)
	if err != nil || !strings.Contains(out, "verified 1 files, 2 records") {
		t.Fatalf("%q %v", out, err)
	}
	keys := t.TempDir()
	if out, _, err := run(t, "", "audit", "keygen", "--out", keys); err != nil || !strings.Contains(out, "public key") {
		t.Fatalf("keygen %q %v", out, err)
	}
	cps := filepath.Join(t.TempDir(), "cp.jsonl")
	if _, _, err := run(t, "", "audit", "checkpoint", "--dir", dir, "--key", filepath.Join(keys, "audit-ed25519.key"),
		"--out", cps); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(keys, "audit-ed25519.pub")
	if out, _, err := run(t, "", "audit", "verify", "--dir", dir, "--checkpoints", cps, "--pubkey", pub); err != nil ||
		!strings.Contains(out, "verified 1 files") {
		t.Fatalf("verify with checkpoints %q %v", out, err)
	}
	if _, _, err := run(t, "", "audit", "verify", "--dir", dir, "--checkpoints", cps); err == nil {
		t.Fatal("checkpoints without a public key accepted")
	}
	out, _, err = run(t, "", "audit", "report", "--dir", dir)
	if err != nil || !strings.Contains(out, "## Tenant `acme`") {
		t.Fatalf("report %q %v", out, err)
	}
	if out, _, err = run(t, "", "audit", "report", "--dir", dir, "--format", "json"); err != nil ||
		!strings.Contains(out, `"tenant":"acme"`) {
		t.Fatalf("json report %q %v", out, err)
	}
	t.Setenv("MASKWIRE_KEYS__MASTER_KEY", strings.Repeat("ab", 32))
	if _, _, err := run(t, "", "audit", "verify", "--dir", dir); err == nil {
		t.Fatal("a different master key must fail verification")
	}
}

func TestHealthcheckCommand(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer ok.Close()
	if _, _, err := run(t, "", "healthcheck", "--url", ok.URL); err != nil {
		t.Fatal(err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	if _, _, err := run(t, "", "healthcheck", "--url", down.URL); err == nil {
		t.Fatal("503 must fail the health check")
	}
}

const reloadYAML = `
keys:
  master_key: "` + "abababababababababababababababababababababababababababababababab" + `"
audit:
  enabled: false
routes:
  - name: chat
    path: /v1/chat/completions
    upstream: %q
    codec: openai
tenants:
  - id: acme
    key_sha256: %q
`

func TestSIGHUPReloadsTenants(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"choices":[]}`)); err != nil {
			t.Errorf("upstream write: %v", err)
		}
	}))
	defer upstream.Close()
	const oldKey, newKey = "mw-before-reload-0000000001", "mw-after-reload-00000000002"
	path := filepath.Join(t.TempDir(), "maskwire.yaml")
	write := func(key string) {
		body := fmt.Sprintf(reloadYAML, upstream.URL, config.HashKey(key))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(oldKey)
	cfg, err := config.Load(path, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	status := func(key string) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/v1/chat/completions",
			strings.NewReader(`{"model":"m","messages":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Maskwire-Key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	stop := watchReload(t.Context(), slog.New(slog.DiscardHandler), a, path)
	defer stop()
	if got := status(oldKey); got != http.StatusOK {
		t.Fatalf("old key before reload: %d", got)
	}
	write(newKey)
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for status(newKey) != http.StatusOK {
		if time.Now().After(deadline) {
			t.Fatal("new key not accepted after SIGHUP")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := status(oldKey); got != http.StatusUnauthorized {
		t.Fatalf("old key after reload: %d", got)
	}
}
