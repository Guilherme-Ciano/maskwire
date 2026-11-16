package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "evaluation.md")
	if err := run([]string{"-out", out, "-docs", "300"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-out", out, "-docs", "300", "-check"}); err != nil {
		t.Fatalf("fresh file: %v", err)
	}
	if err := os.WriteFile(out, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-out", out, "-docs", "300", "-check"}); !errors.Is(err, errOutdated) {
		t.Fatalf("outdated: %v", err)
	}
}
