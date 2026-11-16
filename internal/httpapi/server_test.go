package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func panicked(f func()) (v any) {
	defer func() { v = recover() }()
	f()
	return nil
}

func TestRecovererBeforeAndAfterHeaders(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.New(slog.DiscardHandler)}
	early := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	early.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "internal_error") {
		t.Fatalf("early panic: %d %s", rec.Code, rec.Body.String())
	}
	late := s.recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "partial"); err != nil {
			t.Error(err)
		}
		panic("boom")
	}))
	v := panicked(
		func() {
			late.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody),
			)
		},
	)
	if err, ok := v.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
		t.Fatalf("a panic after headers must abort the response, got %v", v)
	}
	abort := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	v = panicked(func() {
		abort.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody),
		)
	})
	if err, ok := v.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
		t.Fatalf("ErrAbortHandler must be re-panicked, got %v", v)
	}
}

func TestStatusWriterIgnoresInformational(t *testing.T) {
	t.Parallel()
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	sw.WriteHeader(http.StatusEarlyHints)
	sw.WriteHeader(http.StatusTeapot)
	sw.WriteHeader(http.StatusOK)
	if sw.status != http.StatusTeapot {
		t.Fatalf("status %d", sw.status)
	}
	if sw.Unwrap() == nil {
		t.Fatal("Unwrap must expose the writer for ResponseController")
	}
}

func TestWriteBlockedNamesTypesOnly(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeBlocked(rec, []string{"CREDIT_CARD", "IBAN"})
	if rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), `"entities":["CREDIT_CARD","IBAN"]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
