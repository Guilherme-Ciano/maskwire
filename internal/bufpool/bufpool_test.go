package bufpool

import (
	"io"
	"sync"
	"testing"
)

func TestClasses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ n, want int }{
		{0, 0}, {1, 0}, {4096, 0}, {4097, 1}, {8192, 1}, {1 << 26, classes - 1}, {1<<26 + 1, -1},
	} {
		if got := class(tc.n); got != tc.want {
			t.Errorf("class(%d) = %d, want %d", tc.n, got, tc.want)
		}
	}
}

func TestReleaseZeroesAndReuses(t *testing.T) {
	t.Parallel()
	var p Pool
	b := p.Get(100)
	b.B = append(b.B, "secret 529.982.247-25"...)
	backing := b.B[:cap(b.B)]
	b.Release()
	for i, c := range backing {
		if c != 0 {
			t.Fatalf("byte %d not zeroed after release", i)
		}
	}
	if again := p.Get(10); len(again.B) != 0 || cap(again.B) < 10 {
		t.Fatalf("reused buffer not reset: len %d cap %d", len(again.B), cap(again.B))
	}
}

func TestReadersHoldTheBuffer(t *testing.T) {
	t.Parallel()
	var p Pool
	b := p.Get(16)
	b.B = append(b.B, "payload"...)
	r1, r2 := b.Reader(), b.Reader()
	b.Release()
	got, err := io.ReadAll(r1)
	if err != nil || string(got) != "payload" {
		t.Fatalf("reader saw %q after owner release: %v", got, err)
	}
	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	if b.refs.Load() != 1 {
		t.Fatalf("double close released twice: refs %d", b.refs.Load())
	}
	if err := r2.Close(); err != nil {
		t.Fatal(err)
	}
	if b.refs.Load() != 0 {
		t.Fatalf("refs %d after all closed", b.refs.Load())
	}
}

func TestOversizedIsNotPooled(t *testing.T) {
	t.Parallel()
	var p Pool
	b := p.Get(1<<26 + 1)
	b.B = append(b.B, 'x')
	b.Release()
	if b.B[0] != 'x' {
		t.Fatal("oversized buffers are left to the garbage collector")
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()
	var p Pool
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 2000 {
				b := p.Get(i % 9000)
				b.B = append(b.B, byte(i))
				r := b.Reader()
				b.Release()
				if _, err := io.ReadAll(r); err != nil {
					t.Error(err)
				}
				_ = r.Close()
			}
		})
	}
	wg.Wait()
}
