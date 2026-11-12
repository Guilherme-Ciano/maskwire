package audit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

var fixed = time.Date(2026, 11, 11, 14, 3, 22, 0, time.UTC)

func runWriter(t *testing.T, dir string, events []Event) {
	t.Helper()
	w, err := NewWriter(Options{Dir: dir, Now: func() time.Time { return fixed }}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	for i := range events {
		if err := w.Enqueue(t.Context(), &events[i]); err != nil {
			t.Fatal(err)
		}
	}
	for w.Written() < uint64(len(events)) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !w.Healthy() {
		t.Fatal("writer unhealthy")
	}
}

func sampleEvents(n int, tenant string) []Event {
	out := make([]Event, n)
	for i := range out {
		e := Event{
			TS:       fixed,
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
	runWriter(t, dir, append(sampleEvents(5, "acme"), sampleEvents(3, "globex")...))
	runWriter(t, dir, sampleEvents(2, "acme"))
	rep, err := VerifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 || rep.Records != 10 {
		t.Fatalf("report %+v", rep)
	}
	data, err := os.ReadFile(FilePath(dir, "acme", "2026-11-11"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"detections":{"EMAIL":4}`) || strings.Contains(string(data), "@") {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestTamperDetection(t *testing.T) {
	t.Parallel()
	cases := map[string]func([]string) []string{
		"edit":    func(l []string) []string { l[1] = strings.Replace(l[1], `"status":200`, `"status":201`, 1); return l },
		"delete":  func(l []string) []string { return append(l[:1], l[2:]...) },
		"reorder": func(l []string) []string { l[1], l[2] = l[2], l[1]; return l },
		"rehash": func(l []string) []string {
			l[0] = strings.Replace(l[0], `"mode":"enforce"`, `"mode":"observe"`, 1)
			return l
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			runWriter(t, dir, sampleEvents(4, "acme"))
			path := FilePath(dir, "acme", "2026-11-11")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			out := strings.Join(mutate(lines), "\n") + "\n"
			if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = VerifyDir(dir)
			var ce *ChainError
			if !errors.As(err, &ce) || !errors.Is(err, ErrBrokenChain) {
				t.Fatalf("expected chain error, got %v", err)
			}
		})
	}
}

func TestTornTailRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, sampleEvents(2, "acme"))
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
	if _, _, err := VerifyFile(path); err == nil {
		t.Fatal("torn file must fail verification before recovery")
	}
	runWriter(t, dir, sampleEvents(1, "acme"))
	n, _, err := VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("records %d", n)
	}
}

func TestDropMode(t *testing.T) {
	t.Parallel()
	w, err := NewWriter(Options{Dir: t.TempDir(), Buffer: 1, OnFull: Drop}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ev := sampleEvents(1, "acme")[0]
	for range 3 {
		if err := w.Enqueue(t.Context(), &ev); err != nil {
			t.Fatal(err)
		}
	}
	if w.Dropped() != 2 {
		t.Fatalf("dropped %d", w.Dropped())
	}
}

func TestBadTenantRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := NewWriter(Options{Dir: dir}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.chainFor("../escape", fixed); !errors.Is(err, ErrBadTenant) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escape")); err == nil {
		t.Fatal("path traversal created a directory")
	}
}

func TestSafeModel(t *testing.T) {
	t.Parallel()
	if SafeModel("gpt-4.1-mini") != "gpt-4.1-mini" || SafeModel("ana@x.pt") != "invalid" ||
		SafeModel(strings.Repeat("a", 80)) != "invalid" {
		t.Fatal("safe model")
	}
}

func FuzzVerify(f *testing.F) {
	f.Add([]byte(`{"v":1,"prev":"sha256:00"}` + "\n"))
	f.Fuzz(func(_ *testing.T, b []byte) {
		if _, _, err := VerifyReader("fuzz", io.NopCloser(bytes.NewReader(b)), [32]byte{}); err != nil {
			return
		}
	})
}
