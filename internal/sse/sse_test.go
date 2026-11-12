package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func readAll(t *testing.T, in string, maxEvent int) ([]Event, error) {
	t.Helper()
	r := NewReader(strings.NewReader(in), maxEvent)
	var out []Event
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		e.Data = append([]byte(nil), e.Data...)
		e.Name = append([]byte(nil), e.Name...)
		out = append(out, e)
	}
}

func TestReader(t *testing.T) {
	t.Parallel()
	in := ": ping\n\nevent: delta\r\ndata: {\"a\":1}\r\ndata: second\r\n\r\ndata:nospace\n\n\n\nid: 7\ndata: [DONE]\n\ndata: tail-without-blank"
	evs, err := readAll(t, in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 {
		t.Fatalf("got %d events", len(evs))
	}
	if !evs[0].Comment || string(evs[0].Data) != " ping" {
		t.Errorf("comment: %+v", evs[0])
	}
	if string(evs[1].Name) != "delta" || string(evs[1].Data) != "{\"a\":1}\nsecond" {
		t.Errorf("multi-line: %q %q", evs[1].Name, evs[1].Data)
	}
	if string(evs[2].Data) != "nospace" {
		t.Errorf("nospace: %q", evs[2].Data)
	}
	if !evs[3].HasID || string(evs[3].Data) != "[DONE]" {
		t.Errorf("done: %+v", evs[3])
	}
	if string(evs[4].Data) != "tail-without-blank" {
		t.Errorf("tail: %q", evs[4].Data)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	in := "event: x\nid: 1\ndata: a\ndata: b\n\n: c\n\n"
	evs, err := readAll(t, in, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, e := range evs {
		out = AppendEvent(out, e)
	}
	if string(out) != in {
		t.Fatalf("got %q", out)
	}
}

func TestTooLarge(t *testing.T) {
	t.Parallel()
	_, err := readAll(t, "data: "+strings.Repeat("x", 200)+"\n\n", 64)
	if !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func FuzzReader(f *testing.F) {
	f.Add([]byte("data: {}\n\n: x\n\nevent: e\ndata: [DONE]\n\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r := NewReader(strings.NewReader(string(b)), 1024)
		for range 10000 {
			if _, err := r.Next(); err != nil {
				return
			}
		}
		t.Fatal("reader did not terminate")
	})
}
