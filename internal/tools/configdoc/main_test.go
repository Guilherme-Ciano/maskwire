package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
)

func TestEveryKeyDocumented(t *testing.T) {
	d := config.Defaults()
	rows := walk(reflect.TypeOf(d), reflect.ValueOf(d), "", nil)
	doc, err := render(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| `server.addr` | string | `:8090` |",
		"| `server.upstream_timeout` | duration | `1m0s` |",
		"| `detect.regions` | list of string | `BR,PT,ES,DE,FR,IT` |",
		"| `tenants[].by_type.<TYPE>.min_score` | number |  |",
		"| `tenants[].rate_limit.burst` | integer |  |",
	} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestMissingAndStaleDescriptions(t *testing.T) {
	if _, err := render([]row{{key: "server.unknown"}}); !errors.Is(err, errUndocumented) {
		t.Fatalf("undocumented key: %v", err)
	}
	if _, err := render([]row{{key: "server.addr"}}); !errors.Is(err, errStale) {
		t.Fatalf("stale descriptions: %v", err)
	}
}

func TestCheckMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "configuration.md")
	if err := run([]string{"-out", out}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-out", out, "-check"}); err != nil {
		t.Fatalf("fresh file: %v", err)
	}
	if err := os.WriteFile(out, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-out", out, "-check"}); !errors.Is(err, errOutdated) {
		t.Fatalf("outdated file: %v", err)
	}
}
