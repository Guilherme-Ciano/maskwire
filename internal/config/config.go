package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const EnvPrefix = "MASKWIRE_"

var (
	ErrKeyFilePerms = errors.New("master key file must not be accessible by other users")
	ErrKeyEncoding  = errors.New("master key must be hex encoded")
)

type Config struct {
	Server  Server   `koanf:"server"`
	Keys    Keys     `koanf:"keys"`
	Audit   Audit    `koanf:"audit"`
	Detect  Detect   `koanf:"detect"`
	Log     Log      `koanf:"log"`
	Routes  []Route  `koanf:"routes"`
	Tenants []Tenant `koanf:"tenants"`
}

type Server struct {
	Addr               string        `koanf:"addr"`
	AdminAddr          string        `koanf:"admin_addr"`
	ReadHeaderTimeout  time.Duration `koanf:"read_header_timeout"`
	ReadTimeout        time.Duration `koanf:"read_timeout"`
	IdleTimeout        time.Duration `koanf:"idle_timeout"`
	UpstreamTimeout    time.Duration `koanf:"upstream_timeout"`
	StreamIdleTimeout  time.Duration `koanf:"stream_idle_timeout"`
	ShutdownDrainDelay time.Duration `koanf:"shutdown_drain_delay"`
	ShutdownTimeout    time.Duration `koanf:"shutdown_timeout"`
	MaxBodyBytes       int64         `koanf:"max_body_bytes"`
	TenantHeader       string        `koanf:"tenant_header"`
	DefaultTenant      string        `koanf:"default_tenant"`
	Pprof              bool          `koanf:"pprof"`
}

type Keys struct {
	MasterKey     string `koanf:"master_key"`
	MasterKeyFile string `koanf:"master_key_file"`
	Version       int    `koanf:"version"`
}

type Audit struct {
	Enabled bool   `koanf:"enabled"`
	Dir     string `koanf:"dir"`
	OnFull  string `koanf:"on_full"`
	Buffer  int    `koanf:"buffer"`
}

type Detect struct {
	Regions           []string `koanf:"regions"`
	PhoneBudget       int      `koanf:"phone_budget"`
	DisablePhone      bool     `koanf:"disable_phone"`
	KeepReservedEmail bool     `koanf:"keep_reserved_email"`
	MaxEntities       int      `koanf:"max_entities"`
}

type Log struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

type Route struct {
	Name             string `koanf:"name"`
	Path             string `koanf:"path"`
	Upstream         string `koanf:"upstream"`
	Codec            string `koanf:"codec"`
	SynthesizeStream bool   `koanf:"synthesize_stream"`
}

type Rule struct {
	Action   string  `koanf:"action"`
	MinScore float32 `koanf:"min_score"`
}

type Tenant struct {
	ID          string          `koanf:"id"`
	KeySHA256   string          `koanf:"key_sha256"`
	Mode        string          `koanf:"mode"`
	Default     string          `koanf:"default"`
	ByType      map[string]Rule `koanf:"by_type"`
	TokenLength int             `koanf:"token_length"`
	PromptHint  bool            `koanf:"prompt_hint"`
	Foreign     string          `koanf:"foreign_token"`
}

func Defaults() Config {
	return Config{
		Server: Server{
			Addr:               ":8090",
			AdminAddr:          "127.0.0.1:9090",
			ReadHeaderTimeout:  5 * time.Second,
			ReadTimeout:        30 * time.Second,
			IdleTimeout:        120 * time.Second,
			UpstreamTimeout:    60 * time.Second,
			StreamIdleTimeout:  60 * time.Second,
			ShutdownDrainDelay: 5 * time.Second,
			ShutdownTimeout:    20 * time.Second,
			MaxBodyBytes:       4 << 20,
			TenantHeader:       "X-Maskwire-Key",
		},
		Keys:   Keys{Version: 1},
		Audit:  Audit{Enabled: true, Dir: "audit", OnFull: "block", Buffer: 4096},
		Detect: Detect{Regions: []string{"BR", "PT", "ES", "DE", "FR", "IT"}, PhoneBudget: 64, MaxEntities: 4096},
		Log:    Log{Level: "info", Format: "json"},
	}
}

func envKey(k, v string) (key string, value any) {
	k = strings.TrimPrefix(k, EnvPrefix)
	k = strings.ToLower(strings.ReplaceAll(k, "__", "."))
	if k == "detect.regions" {
		return k, strings.Split(v, ",")
	}
	return k, v
}

func Load(path string, environ func() []string) (Config, error) {
	k := koanf.New(".")
	if err := k.Load(structs.Provider(Defaults(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}
	if path != "" {
		if err := k.Load(file.Provider(filepath.Clean(path)), yaml.Parser()); err != nil {
			return Config{}, fmt.Errorf("load config file: %w", err)
		}
	}
	if err := k.Load(
		env.Provider(".", env.Opt{Prefix: EnvPrefix, TransformFunc: envKey, EnvironFunc: environ}),
		nil,
	); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}
	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

func (c *Config) MasterKey() ([]byte, error) {
	if c.Keys.MasterKeyFile != "" {
		st, err := os.Stat(filepath.Clean(c.Keys.MasterKeyFile))
		if err != nil {
			return nil, fmt.Errorf("stat master key file: %w", err)
		}
		if st.Mode().Perm()&0o007 != 0 {
			return nil, ErrKeyFilePerms
		}
		b, err := os.ReadFile(filepath.Clean(c.Keys.MasterKeyFile))
		if err != nil {
			return nil, fmt.Errorf("read master key file: %w", err)
		}
		return decodeKey(strings.TrimSpace(string(b)))
	}
	return decodeKey(c.Keys.MasterKey)
}

func decodeKey(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, ErrKeyEncoding
	}
	if len(b) < token.MinMasterKeyLen {
		return nil, token.ErrShortMasterKey
	}
	return b, nil
}

func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (c *Config) Validate() error {
	var errs []error
	if c.Server.Addr == "" {
		errs = append(errs, errors.New("server.addr is required"))
	}
	if c.Server.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("server.max_body_bytes must be positive"))
	}
	if c.Server.TenantHeader == "" {
		errs = append(errs, errors.New("server.tenant_header is required"))
	}
	if key, err := c.MasterKey(); err != nil {
		errs = append(errs, fmt.Errorf("keys: %w", err))
	} else {
		clear(key)
	}
	if c.Keys.Version < 1 || c.Keys.Version > 65535 {
		errs = append(errs, errors.New("keys.version must be in 1..65535"))
	}
	if c.Audit.Enabled && c.Audit.Dir == "" {
		errs = append(errs, errors.New("audit.dir is required when audit is enabled"))
	}
	if c.Audit.OnFull != "block" && c.Audit.OnFull != "drop" {
		errs = append(errs, fmt.Errorf("audit.on_full must be block or drop, got %q", c.Audit.OnFull))
	}
	if len(c.Routes) == 0 {
		errs = append(errs, errors.New("at least one route is required"))
	}
	paths := map[string]bool{}
	for i, r := range c.Routes {
		errs = append(errs, r.validate(i, paths)...)
	}
	if len(c.Tenants) == 0 {
		errs = append(errs, errors.New("at least one tenant is required"))
	}
	ids := map[string]bool{}
	for i, t := range c.Tenants {
		errs = append(errs, t.validate(i, ids)...)
	}
	if c.Server.DefaultTenant != "" && !ids[c.Server.DefaultTenant] {
		errs = append(errs, fmt.Errorf("server.default_tenant %q is not a configured tenant", c.Server.DefaultTenant))
	}
	return errors.Join(errs...)
}

func (r Route) validate(i int, paths map[string]bool) []error {
	var errs []error
	if !strings.HasPrefix(r.Path, "/") {
		errs = append(errs, fmt.Errorf("routes[%d].path must start with /", i))
	}
	if paths[r.Path] {
		errs = append(errs, fmt.Errorf("routes[%d].path %q is duplicated", i, r.Path))
	}
	paths[r.Path] = true
	u, err := url.Parse(r.Upstream)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("routes[%d].upstream must be an absolute http(s) URL", i))
	}
	switch r.Codec {
	case "openai", "openai-responses", "openai-embeddings", "anthropic", "passthrough":
	default:
		errs = append(errs, fmt.Errorf(
			"routes[%d].codec must be openai, openai-responses, openai-embeddings, anthropic or passthrough, got %q",
			i,
			r.Codec,
		))
	}
	return errs
}

func (t Tenant) validate(i int, ids map[string]bool) []error {
	var errs []error
	if !audit.ValidTenant(t.ID) || ids[t.ID] {
		errs = append(errs, fmt.Errorf("tenants[%d].id must be unique, at most 64 chars of [A-Za-z0-9._-]", i))
	}
	ids[t.ID] = true
	if b, err := hex.DecodeString(t.KeySHA256); err != nil || len(b) != sha256.Size {
		errs = append(errs, fmt.Errorf("tenants[%d].key_sha256 must be a hex sha256 digest", i))
	}
	if _, err := t.Policy(); err != nil {
		errs = append(errs, fmt.Errorf("tenants[%d]: %w", i, err))
	}
	if t.TokenLength != 0 && (t.TokenLength < token.MinLength || t.TokenLength > token.MaxLength) {
		errs = append(
			errs,
			fmt.Errorf("tenants[%d].token_length must be in %d..%d", i, token.MinLength, token.MaxLength),
		)
	}
	if t.Foreign != "" && t.Foreign != "generic" && t.Foreign != "keep" {
		errs = append(errs, fmt.Errorf("tenants[%d].foreign_token must be generic or keep", i))
	}
	return errs
}

func (t Tenant) Policy() (*policy.Policy, error) {
	mode, err := policy.ParseMode(t.Mode)
	if err != nil {
		return nil, err
	}
	def, err := policy.ParseAction(t.Default)
	if err != nil {
		return nil, err
	}
	p := policy.New(mode, def)
	for name, r := range t.ByType {
		typ, ok := entity.Parse(strings.ToUpper(name))
		if !ok {
			return nil, fmt.Errorf("unknown entity type %q", name)
		}
		a, err := policy.ParseAction(r.Action)
		if err != nil {
			return nil, err
		}
		p.Set(typ, policy.Rule{Action: a, MinScore: r.MinScore})
	}
	return p, nil
}
