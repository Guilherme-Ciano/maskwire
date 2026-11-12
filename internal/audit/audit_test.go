package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

var (
	fixed   = time.Date(2026, 11, 11, 14, 3, 22, 0, time.UTC)
	testKey = bytes.Repeat([]byte{9}, 32)
)

func sealer() *Sealer {
	return NewSealer(testKey)
}

func newWriter(t *testing.T, dir string, o Options) *Writer {
	t.Helper()
	o.Dir, o.Key = dir, testKey
	if o.Now == nil {
		o.Now = func() time.Time { return fixed }
	}
	w, err := NewWriter(o, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func runWriter(t *testing.T, dir string, events []Event) *Writer {
	t.Helper()
	w := newWriter(t, dir, Options{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	for i := range events {
		if err := w.Enqueue(t.Context(), &events[i]); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !w.Healthy() {
		t.Fatal("writer unhealthy")
	}
	return w
}

func sampleEvents(n int, tenant string, ts time.Time) []Event {
	out := make([]Event, n)
	for i := range out {
		e := Event{
			TS:       ts,
			Tenant:   tenant,
			Route:    "openai",
			Upstream: "mockllm:9000",
			Model:    "gpt-4.1-mini",
			Mode:     "enforce",
			Status:   200,
		}
		e.Detected[entity.Email] = uint32(i)
		e.Mask = uint32(i)
		out[i] = e
	}
	return out
}

func TestChainAndVerify(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, append(sampleEvents(5, "acme", fixed), sampleEvents(3, "globex", fixed)...))
	runWriter(t, dir, sampleEvents(2, "acme", fixed))
	runWriter(t, dir, sampleEvents(2, "acme", fixed.Add(24*time.Hour)))
	rep, err := VerifyDir(dir, sealer())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 3 || rep.Records != 5+2+1+3+1+2+1 {
		t.Fatalf("report %+v", rep)
	}
	data, err := os.ReadFile(FilePath(dir, "acme", "2026-11-11"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"detections":{"EMAIL":4}`) || strings.Contains(string(data), "@") {
		t.Fatalf("unexpected content: %s", data)
	}
	if _, err := VerifyDir(dir, NewSealer(bytes.Repeat([]byte{1}, 32))); !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("wrong key must fail, got %v", err)
	}
}

func mutateFile(t *testing.T, path string, mutate func([]string) []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	out := mutate(lines)
	content := ""
	if len(out) > 0 {
		content = strings.Join(out, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTamperDetection(t *testing.T) {
	t.Parallel()
	day1, day2 := "2026-11-11", "2026-11-12"
	cases := map[string]struct {
		file   string
		mutate func([]string) []string
	}{
		"edit": {
			day1,
			func(l []string) []string { l[1] = strings.Replace(l[1], `"status":200`, `"status":201`, 1); return l },
		},
		"delete line":    {day1, func(l []string) []string { return append(l[:1], l[2:]...) }},
		"reorder":        {day1, func(l []string) []string { l[1], l[2] = l[2], l[1]; return l }},
		"truncate day 1": {day1, func(l []string) []string { return l[:2] }},
		"drop open":      {day2, func(l []string) []string { return l[1:] }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			runWriter(t, dir, sampleEvents(4, "acme", fixed))
			runWriter(t, dir, sampleEvents(2, "acme", fixed.Add(24*time.Hour)))
			mutateFile(t, FilePath(dir, "acme", tc.file), tc.mutate)
			_, err := VerifyDir(dir, sealer())
			var ce *ChainError
			if !errors.As(err, &ce) || !errors.Is(err, ErrBrokenChain) {
				t.Fatalf("expected chain error, got %v", err)
			}
		})
	}
}

func TestMissingPreviousDayDetected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, sampleEvents(2, "acme", fixed))
	runWriter(t, dir, sampleEvents(2, "acme", fixed.Add(24*time.Hour)))
	if err := os.Remove(FilePath(dir, "acme", "2026-11-11")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDir(dir, sealer()); !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("deleted day must be detected, got %v", err)
	}
}

func TestTornTailRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, sampleEvents(2, "acme", fixed))
	path := FilePath(dir, "acme", "2026-11-11")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"v":1,"type":"request","id":"to`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFile(path, sealer()); err == nil {
		t.Fatal("torn file must fail verification before recovery")
	}
	runWriter(t, dir, sampleEvents(1, "acme", fixed))
	res, err := VerifyFile(path, sealer())
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 6 {
		t.Fatalf("records %d", res.Records)
	}
}

func TestDropMode(t *testing.T) {
	t.Parallel()
	w := newWriter(t, t.TempDir(), Options{Buffer: 1, OnFull: Drop})
	ev := sampleEvents(1, "acme", fixed)[0]
	for range 3 {
		if err := w.Enqueue(t.Context(), &ev); err != nil {
			t.Fatal(err)
		}
	}
	if w.Dropped() != 2 {
		t.Fatalf("dropped %d", w.Dropped())
	}
}

func TestInvalidTenantDoesNotPoisonWriter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := runWriter(t, dir, append(sampleEvents(1, "../escape", fixed), sampleEvents(1, "acme", fixed)...))
	if w.Dropped() != 1 || w.Written() != 1 || !w.Healthy() {
		t.Fatalf("dropped=%d written=%d healthy=%v", w.Dropped(), w.Written(), w.Healthy())
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escape")); err == nil {
		t.Fatal("path traversal created a directory")
	}
}

func TestEnqueueAfterCloseIsRejected(t *testing.T) {
	t.Parallel()
	w := runWriter(t, t.TempDir(), nil)
	ev := sampleEvents(1, "acme", fixed)[0]
	if err := w.Enqueue(t.Context(), &ev); !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestValidTenantAndSafeModel(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]bool{"acme": true, "acme-eu_1.x": true, "acme:prod": false, "..": false, strings.Repeat("a", 65): false} {
		if ValidTenant(id) != want {
			t.Errorf("ValidTenant(%q) != %v", id, want)
		}
	}
	if SafeModel("gpt-4.1-mini") != "gpt-4.1-mini" || SafeModel("ana@x.pt") != "invalid" ||
		SafeModel(strings.Repeat("a", 80)) != "invalid" {
		t.Fatal("safe model")
	}
}

func FuzzVerify(f *testing.F) {
	f.Add([]byte(`{"v":1,"prev":"sha256:00"}` + "\n"))
	s := sealer()
	f.Fuzz(func(_ *testing.T, b []byte) {
		if _, err := VerifyReader("fuzz", bytes.NewReader(b), s, [32]byte{}); err != nil {
			return
		}
	})
}
