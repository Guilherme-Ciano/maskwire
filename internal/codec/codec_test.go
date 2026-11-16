package codec

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

type slowReader struct {
	delay time.Duration
	n     int
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	if r.n == 0 {
		return 0, io.EOF
	}
	r.n--
	return copy(p, "x"), nil
}

func TestIdleReaderFiresOnlyWhileWaitingForUpstream(t *testing.T) {
	t.Parallel()
	var fired atomic.Int32
	r := WatchIdle(&slowReader{delay: 5 * time.Millisecond, n: 3}, 50*time.Millisecond, func() { fired.Add(1) })
	buf := make([]byte, 4)
	for {
		if _, err := r.Read(buf); err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	r.Stop()
	if fired.Load() != 0 {
		t.Fatal("time spent outside Read must not count as a stall")
	}
	slow := WatchIdle(&slowReader{delay: 60 * time.Millisecond, n: 1}, 20*time.Millisecond, func() { fired.Add(1) })
	if _, err := slow.Read(buf); err != nil {
		t.Fatal(err)
	}
	slow.Stop()
	if fired.Load() != 1 {
		t.Fatalf("a slow upstream read must fire once, fired %d", fired.Load())
	}
	plain := WatchIdle(&slowReader{n: 1}, 0, nil)
	if _, err := plain.Read(buf); err != nil {
		t.Fatal(err)
	}
	plain.Stop()
}

func TestSchemaText(t *testing.T) {
	t.Parallel()
	f := func(names ...string) []jsonx.Frame {
		out := make([]jsonx.Frame, 0, len(names))
		for _, n := range names {
			if n == "#" {
				out = append(out, jsonx.Frame{Array: true})
				continue
			}
			out = append(out, jsonx.Frame{Name: []byte(n)})
		}
		return out
	}
	cases := []struct {
		path []jsonx.Frame
		want bool
	}{
		{f("tools", "#", "parameters", "description"), true},
		{f("tools", "#", "parameters", "properties", "to", "enum", "#"), true},
		{f("tools", "#", "parameters", "properties", "to", "pattern"), false},
		{f("tools", "#", "parameters", "properties", "to", "type"), false},
		{f("tools", "#", "parameters", "properties", "description", "type"), false},
		{f("tools", "#", "parameters"), false},
	}
	for _, tc := range cases {
		root := SchemaRoot(tc.path, "parameters")
		if got := SchemaText(tc.path, root); got != tc.want {
			t.Errorf("%v: got %v", tc.path, got)
		}
	}
}
