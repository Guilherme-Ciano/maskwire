package policy

import (
	"fmt"
	"strings"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type Action uint8

const (
	Mask Action = iota
	Redact
	Block
	Allow
)

func (a Action) String() string {
	switch a {
	case Mask:
		return "mask"
	case Redact:
		return "redact"
	case Block:
		return "block"
	case Allow:
		return "allow"
	}
	return "unknown"
}

func ParseAction(s string) (Action, error) {
	switch strings.ToLower(s) {
	case "mask", "":
		return Mask, nil
	case "redact":
		return Redact, nil
	case "block":
		return Block, nil
	case "allow":
		return Allow, nil
	}
	return Mask, fmt.Errorf("unknown action %q", s)
}

type Mode uint8

const (
	Enforce Mode = iota
	Observe
)

func (m Mode) String() string {
	if m == Observe {
		return "observe"
	}
	return "enforce"
}

func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "enforce", "":
		return Enforce, nil
	case "observe":
		return Observe, nil
	}
	return Enforce, fmt.Errorf("unknown mode %q", s)
}

type Rule struct {
	Action   Action
	MinScore float32
}

type Policy struct {
	Mode  Mode
	rules [entity.Count]Rule
}

func New(mode Mode, def Action) *Policy {
	p := &Policy{Mode: mode}
	for i := range p.rules {
		p.rules[i] = Rule{Action: def}
	}
	p.rules[entity.IPPrivate] = Rule{Action: Allow}
	return p
}

func (p *Policy) Set(t entity.Type, r Rule) {
	if t.Valid() {
		p.rules[t] = r
	}
}

func (p *Policy) Rule(t entity.Type) Rule {
	if !t.Valid() {
		return Rule{Action: Allow}
	}
	return p.rules[t]
}

func (p *Policy) Decide(t entity.Type, score float32) Action {
	r := p.Rule(t)
	if score < r.MinScore {
		return Allow
	}
	return r.Action
}
