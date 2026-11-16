package eval

import (
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func TestDetectionQuality(t *testing.T) {
	t.Parallel()
	results := Run(1, 6000, []float32{0, 0.7})
	for _, r := range results {
		for _, ty := range r.Types {
			l := ty.ByPres[Labelled]
			if l.Gold > 0 && float64(l.Hit)/float64(l.Gold) < 0.98 {
				t.Errorf("min_score %.1f %s labelled recall %d/%d", r.Threshold, ty.Type, l.Hit, l.Gold)
			}
			if l.Gold > 0 && float64(l.Leak)/float64(l.Gold) > 0.02 {
				t.Errorf("min_score %.1f %s labelled leaks %d/%d", r.Threshold, ty.Type, l.Leak, l.Gold)
			}
			if ty.Type == entity.Email && ty.FP > 0 {
				t.Errorf("e-mail false positives %d", ty.FP)
			}
		}
		for _, n := range r.Negatives {
			limit := 0.005
			if strings.HasPrefix(n.Kind, "random") {
				limit = 0.1
				if r.Threshold >= 0.7 {
					limit = 0.005
				}
			}
			if float64(n.Flagged)/float64(n.Seen) > limit {
				t.Errorf("min_score %.1f negative %q flagged %d/%d", r.Threshold, n.Kind, n.Flagged, n.Seen)
			}
		}
	}
}

func TestDeterministic(t *testing.T) {
	t.Parallel()
	a := Markdown(3, Run(3, 500, []float32{0}))
	if b := Markdown(3, Run(3, 500, []float32{0})); a != b {
		t.Fatal("same seed gives different reports")
	}
	if !strings.Contains(a, "| `CPF` |") {
		t.Fatal("report misses CPF row")
	}
}
