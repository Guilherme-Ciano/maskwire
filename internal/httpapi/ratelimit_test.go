package httpapi

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	if NewLimiter(0, 5) != nil {
		t.Fatal("zero rate must disable the limiter")
	}
	var nilLimiter *Limiter
	if ok, _ := nilLimiter.Allow(); !ok {
		t.Fatal("nil limiter must allow")
	}
	clock := time.Unix(1000, 0)
	l := NewLimiter(2, 3)
	l.now = func() time.Time { return clock }
	for i := range 3 {
		if ok, _ := l.Allow(); !ok {
			t.Fatalf("burst request %d refused", i)
		}
	}
	ok, retry := l.Allow()
	if ok || retry != 500*time.Millisecond {
		t.Fatalf("over burst: ok=%v retry=%v", ok, retry)
	}
	clock = clock.Add(500 * time.Millisecond)
	if ok, _ := l.Allow(); !ok {
		t.Fatal("refill after 500ms at 2 rps")
	}
	clock = clock.Add(time.Hour)
	for i := range 3 {
		if ok, _ := l.Allow(); !ok {
			t.Fatalf("refilled burst request %d refused", i)
		}
	}
	if ok, _ := l.Allow(); ok {
		t.Fatal("tokens exceeded burst after idle")
	}
	if d := NewLimiter(2.5, 0); d.burst != 3 {
		t.Fatalf("default burst %v", d.burst)
	}
}
