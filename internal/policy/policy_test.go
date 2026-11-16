package policy

import (
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]Action{"": Mask, "MASK": Mask, "redact": Redact, "block": Block, "Allow": Allow} {
		if got, err := ParseAction(in); err != nil || got != want {
			t.Errorf("ParseAction(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseAction("hash"); err == nil {
		t.Error("unknown action must fail")
	}
	if m, err := ParseMode("observe"); err != nil || m != Observe || m.String() != "observe" {
		t.Errorf("observe: %v %v", m, err)
	}
	if _, err := ParseMode("dry-run"); err == nil {
		t.Error("unknown mode must fail")
	}
}

func TestDecide(t *testing.T) {
	t.Parallel()
	p := New(Enforce, Mask)
	if p.Decide(entity.IPPrivate, 1) != Allow {
		t.Error("private IPs are allowed by default")
	}
	p.Set(entity.PTNIF, Rule{Action: Mask, MinScore: 0.7})
	if p.Decide(entity.PTNIF, 0.6) != Allow || p.Decide(entity.PTNIF, 0.9) != Mask {
		t.Error("min_score must gate the action")
	}
	p.Set(entity.Unknown, Rule{Action: Block})
	if p.Decide(entity.Unknown, 1) != Allow || p.Rule(entity.Type(200)).Action != Allow {
		t.Error("invalid types must never be acted on")
	}
	for _, a := range []Action{Mask, Redact, Block, Allow, Action(9)} {
		if a.String() == "" {
			t.Error("every action has a name")
		}
	}
}
