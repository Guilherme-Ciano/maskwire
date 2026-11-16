package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

const validYAML = `
server:
  addr: ":7000"
  upstream_timeout: 3s
keys:
  master_key: "%s"
routes:
  - name: chat
    path: /v1/chat/completions
    upstream: http://upstream:9000
    codec: openai
tenants:
  - id: acme
    key_sha256: "%s"
    by_type:
      CREDIT_CARD: {action: block}
      pt_nif: {action: mask, min_score: 0.7}
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "maskwire.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func fill(key string) string {
	return strings.Replace(strings.Replace(validYAML, `"%s"`, `"`+key+`"`, 1), `"%s"`, `"`+HashKey("k")+`"`, 1)
}

func TestLoadLayersDefaultsFileAndEnv(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, fill(strings.Repeat("ab", 32)))
	cfg, err := Load(path, func() []string {
		return []string{"MASKWIRE_SERVER__ADMIN_ADDR=127.0.0.1:9999", "MASKWIRE_DETECT__REGIONS=PT,BR", "OTHER=x"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != ":7000" || cfg.Server.UpstreamTimeout != 3*time.Second {
		t.Fatalf("file values not applied: %+v", cfg.Server)
	}
	if cfg.Server.AdminAddr != "127.0.0.1:9999" || strings.Join(cfg.Detect.Regions, ",") != "PT,BR" {
		t.Fatalf("env values not applied: %q %v", cfg.Server.AdminAddr, cfg.Detect.Regions)
	}
	if cfg.Server.MaxBodyBytes != Defaults().Server.MaxBodyBytes {
		t.Fatal("defaults lost")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := cfg.Tenants[0].Policy()
	if err != nil {
		t.Fatal(err)
	}
	if r := p.Rule(entity.CreditCard); r.Action.String() != "block" {
		t.Fatalf("CREDIT_CARD rule %+v", r)
	}
}

func TestValidateCollectsErrors(t *testing.T) {
	t.Parallel()
	cfg := Defaults()
	cfg.Keys.MasterKey = "abcd"
	cfg.Audit.OnFull = "maybe"
	cfg.Routes = []Route{
		{Name: "a", Path: "v1", Upstream: "ftp://x", Codec: "grpc"},
		{Name: "b", Path: "/dup", Upstream: "http://x", Codec: "openai"},
		{Name: "c", Path: "/dup", Upstream: "http://x", Codec: "openai"},
	}
	cfg.Tenants = []Tenant{
		{ID: "acme:prod", KeySHA256: "nothex", TokenLength: 3, Foreign: "restore"},
		{ID: "ok", KeySHA256: HashKey("x"), ByType: map[string]Rule{"NAME": {Action: "mask"}}},
	}
	cfg.Server.DefaultTenant = "ghost"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{
		"keys:", "on_full", "routes[0].path", "routes[0].upstream", "routes[0].codec",
		"routes[2].path", "tenants[0].id", "tenants[0].key_sha256", "token_length", "foreign_token", "unknown entity type",
		"default_tenant",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestMasterKeyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := filepath.Join(dir, "good.key")
	open := filepath.Join(dir, "open.key")
	if err := os.WriteFile(good, []byte(strings.Repeat("cd", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(open, []byte(strings.Repeat("cd", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.Keys.MasterKeyFile = good
	if k, err := cfg.MasterKey(); err != nil || len(k) != 32 {
		t.Fatalf("good key: %v %d", err, len(k))
	}
	cfg.Keys.MasterKeyFile = open
	if _, err := cfg.MasterKey(); !errors.Is(err, ErrKeyFilePerms) {
		t.Fatalf("world-readable key file must be refused, got %v", err)
	}
	cfg.Keys.MasterKeyFile = ""
	cfg.Keys.MasterKey = "zz"
	if _, err := cfg.MasterKey(); !errors.Is(err, ErrKeyEncoding) {
		t.Fatalf("non-hex key: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), func() []string { return nil }); err == nil {
		t.Fatal("expected error for missing file")
	}
}
