package detect

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestRulesFitKeywordMask(t *testing.T) {
	t.Parallel()
	if n := len(ruleSet()); n > maxKeyedRules {
		t.Fatalf("%d rules exceed the %d-bit keyword mask", n, maxKeyedRules)
	}
}

func TestKeywordMaskBoundaries(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	r := d.rules.byType[d.rules.all[0].typ]
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"o " + string(r.keys[0]) + " ", true},
		{"o x" + string(r.keys[0]) + " ", false},
	} {
		s := scanner{d: d, text: []byte(tc.text)}
		if got := s.keyword(len(tc.text), r); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.text, got, tc.want)
		}
	}
	s := scanner{d: d, text: []byte("ligue para ")}
	if !s.phoneContext(len(s.text)) {
		t.Error("phone keyword not found")
	}
	s = scanner{d: d, text: []byte("intel ")}
	if s.phoneContext(len(s.text)) {
		t.Error("phone keyword matched inside a word")
	}
}

func TestRegionMaskMatchesWindowMask(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	pieces := []string{
		"cpf", "CPF", "tel", "Telefone", "ein", "Einstein", "nif", "iban", "ssn", "ligue", "x", "a", "É", "ç",
		" ", " ", ".", ":", "-", "/", "(", "+", "1", "42", "529", "\n", "nº", "dob", "birth",
	}
	rng := rand.New(rand.NewPCG(7, 11))
	checked, found := 0, 0
	for range 4000 {
		var b strings.Builder
		for range 4 + rng.IntN(60) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		text := []byte(b.String())
		a := rng.IntN(len(text))
		e := a + rng.IntN(len(text)-a+1)
		s := scanner{d: d, text: text, run: 1, runID: 1}
		s.groups[0] = group{start: a, end: e}
		for pos := max(a-1, 0); pos <= e; pos++ {
			got, ok := s.regionMask(pos)
			if !ok {
				continue
			}
			want := s.lower(pos)
			checked++
			if want.rules != 0 || want.phone {
				found++
			}
			if got.rules != want.rules || got.phone != want.phone {
				t.Fatalf(
					"%q pos %d: region %x/%v, window %x/%v",
					text,
					pos,
					got.rules,
					got.phone,
					want.rules,
					want.phone,
				)
			}
		}
	}
	if checked < 10000 || found < 1000 {
		t.Fatalf("too few comparisons: %d checked, %d with a keyword", checked, found)
	}
}
