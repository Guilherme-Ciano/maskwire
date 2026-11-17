package detect

import (
	"sync"
	"testing"
)

func TestPhoneVerdictsSurviveRequests(t *testing.T) {
	t.Parallel()
	d := New(Options{PhoneBudget: 1, Regions: []string{"PT"}})
	first := d.NewPhoneState()
	d.DetectWith(nil, []byte("ligue 912 345 678"), first)
	if first.Budget != 0 {
		t.Fatalf("the first sighting must parse, budget left %d", first.Budget)
	}
	next := d.NewPhoneState()
	spans := d.DetectWith(nil, []byte("ligue 912 345 678"), next)
	if len(spans) != 1 || spans[0].Score < 0.85 {
		t.Fatalf("a verdict from an earlier request must be reused: %+v", spans)
	}
	if next.Budget != 1 {
		t.Fatalf("a cached verdict must not spend budget, left %d", next.Budget)
	}
}

func TestPhoneCacheKeepsInvalidVerdicts(t *testing.T) {
	t.Parallel()
	d := New(Options{Regions: []string{"PT"}})
	for range 2 {
		if spans := d.Detect(nil, []byte("ligue 000 000 000")); len(spans) != 0 {
			t.Fatalf("invalid number masked: %+v", spans)
		}
	}
}

func TestPhoneCacheSeparatesPlus(t *testing.T) {
	t.Parallel()
	c := newPhoneCache()
	raw := []byte("351912345678")
	if c.key(raw, true) == c.key(raw, false) {
		t.Fatal("international and national readings must not share a verdict")
	}
}

func TestPhoneCacheIsBounded(t *testing.T) {
	t.Parallel()
	c := newPhoneCache()
	for i := range phoneCacheSize * 2 {
		c.put(uint64(i), true)
	}
	if n := len(c.known); n > phoneCacheSize {
		t.Fatalf("cache grew to %d entries", n)
	}
}

func TestPhoneCacheConcurrent(t *testing.T) {
	t.Parallel()
	d := New(Options{Regions: []string{"BR", "PT"}})
	texts := []string{"ligue 912 345 678", "tel +55 11 98765-4321", "telefone (11) 98765-4321"}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				if spans := d.Detect(nil, []byte(texts[(i+j)%len(texts)])); len(spans) != 1 {
					t.Errorf("phone not found: %+v", spans)
					return
				}
			}
		})
	}
	wg.Wait()
}
