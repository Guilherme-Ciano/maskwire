package eval

import (
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type Presentation uint8

const (
	Labelled Presentation = iota
	Formatted
	Bare
	presentations
)

func (p Presentation) String() string {
	switch p {
	case Labelled:
		return "labelled"
	case Formatted:
		return "formatted"
	case Bare:
		return "bare"
	case presentations:
	}
	return "?"
}

type gold struct {
	start, end int
	typ        entity.Type
	pres       Presentation
}

type negSpan struct {
	start, end int
	kind       string
}

type doc struct {
	text []byte
	gold []gold
	negs []negSpan
}

func neutral() []string {
	return []string{"ref", "see", "nota:", "item", "segue", "siehe", "voir", "vedi"}
}

func (g *gen) doc(values []func() value, negs []func() negative) doc {
	var d doc
	add := func(s string) {
		if len(d.text) > 0 {
			d.text = append(d.text, ' ')
		}
		d.text = append(d.text, s...)
	}
	for range 1 + g.r.IntN(2) {
		n := negs[g.r.IntN(len(negs))]()
		add(g.pick("Obrigado.", "Please check.", "Danke.", "Merci,", "ok"))
		add("")
		start := len(d.text)
		d.text = append(d.text, n.text...)
		d.negs = append(d.negs, negSpan{start, len(d.text), n.kind})
	}
	v := values[g.r.IntN(len(values))]()
	pres := []Presentation{Labelled, Formatted, Bare}[g.r.IntN(3)]
	text := v.formatted
	switch pres {
	case Labelled:
		add(v.keyword)
	case Formatted:
		add(g.pick(neutral()...))
	case Bare:
		add(g.pick(neutral()...))
		text = v.compact
	case presentations:
	}
	add("")
	start := len(d.text)
	d.text = append(d.text, text...)
	d.gold = append(d.gold, gold{start, len(d.text), v.typ, pres})
	add(g.pick("hoje.", "today.", "bitte.", "merci.", "grazie."))
	return d
}

type Counts struct {
	Gold, Hit, Leak int
}

type TypeResult struct {
	Type   entity.Type
	ByPres [presentations]Counts
	Pred   int
	FP     int
}

type NegResult struct {
	Kind    string
	Seen    int
	Flagged int
}

type Result struct {
	Docs      int
	Threshold float32
	Types     []TypeResult
	Negatives []NegResult
}

func overlaps(a0, a1, b0, b1 int) bool {
	return a0 < b1 && b0 < a1
}

func covered(spans []entity.Span, start, end int) bool {
	pos := start
	for pos < end {
		next := pos
		for _, s := range spans {
			if s.Start <= pos && s.End > pos {
				next = max(next, s.End)
			}
		}
		if next == pos {
			return false
		}
		pos = next
	}
	return true
}

type scorer struct {
	th     float32
	types  map[entity.Type]*TypeResult
	negs   map[string]*NegResult
	spans  []entity.Span
	detect func(dst []entity.Span, text []byte) []entity.Span
}

func (sc *scorer) typ(t entity.Type) *TypeResult {
	if sc.types[t] == nil {
		sc.types[t] = &TypeResult{Type: t}
	}
	return sc.types[t]
}

func sameType(s entity.Span, gd gold) bool {
	return s.Type.Label() == gd.typ.Label() && overlaps(s.Start, s.End, gd.start, gd.end)
}

func (sc *scorer) score(d *doc) {
	sc.spans = sc.detect(sc.spans[:0], d.text)
	sc.spans = slices.DeleteFunc(sc.spans, func(s entity.Span) bool { return s.Score < sc.th })
	for _, gd := range d.gold {
		c := &sc.typ(gd.typ).ByPres[gd.pres]
		c.Gold++
		if slices.ContainsFunc(sc.spans, func(s entity.Span) bool { return sameType(s, gd) }) {
			c.Hit++
		}
		if !covered(sc.spans, gd.start, gd.end) {
			c.Leak++
		}
	}
	for _, s := range sc.spans {
		t := sc.typ(s.Type)
		t.Pred++
		if !slices.ContainsFunc(d.gold, func(gd gold) bool { return sameType(s, gd) }) {
			t.FP++
		}
	}
	for _, n := range d.negs {
		nr := sc.negs[n.kind]
		if nr == nil {
			nr = &NegResult{Kind: n.kind}
			sc.negs[n.kind] = nr
		}
		nr.Seen++
		if slices.ContainsFunc(sc.spans, func(s entity.Span) bool { return overlaps(s.Start, s.End, n.start, n.end) }) {
			nr.Flagged++
		}
	}
}

func (sc *scorer) result(docs int) Result {
	res := Result{Docs: docs, Threshold: sc.th}
	for _, t := range sc.types {
		res.Types = append(res.Types, *t)
	}
	slices.SortFunc(res.Types, func(a, b TypeResult) int { return int(a.Type) - int(b.Type) })
	for _, n := range sc.negs {
		res.Negatives = append(res.Negatives, *n)
	}
	slices.SortFunc(res.Negatives, func(a, b NegResult) int { return strings.Compare(a.Kind, b.Kind) })
	return res
}

func corpus(seed uint64, docs int) []doc {
	g := &gen{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
	values, negs := g.allValues(), g.negatives()
	out := make([]doc, docs)
	for i := range out {
		out[i] = g.doc(values, negs)
	}
	return out
}

func runWith(
	c []doc,
	thresholds []float32,
	detectFn func(i int, dst []entity.Span, text []byte) []entity.Span,
) []Result {
	out := make([]Result, 0, len(thresholds))
	for _, th := range thresholds {
		sc := &scorer{th: th, types: map[entity.Type]*TypeResult{}, negs: map[string]*NegResult{}}
		for i := range c {
			sc.detect = func(dst []entity.Span, text []byte) []entity.Span { return detectFn(i, dst, text) }
			sc.score(&c[i])
		}
		out = append(out, sc.result(len(c)))
	}
	return out
}

func Run(seed uint64, docs int, thresholds []float32) []Result {
	det := detect.New(detect.Options{Names: true})
	return runWith(corpus(seed, docs), thresholds, func(_ int, dst []entity.Span, text []byte) []entity.Span {
		return det.Detect(dst, text)
	})
}

type ExportDoc struct {
	Text string `json:"text"`
}

func Export(seed uint64, docs int) []ExportDoc {
	c := corpus(seed, docs)
	out := make([]ExportDoc, len(c))
	for i := range c {
		out[i] = ExportDoc{Text: string(c[i].text)}
	}
	return out
}

type ExternalSpan struct {
	Start int     `json:"start"`
	End   int     `json:"end"`
	Type  string  `json:"type"`
	Score float32 `json:"score"`
}

func RunExternal(seed uint64, docs int, thresholds []float32, preds [][]ExternalSpan) []Result {
	return runWith(corpus(seed, docs), thresholds, func(i int, dst []entity.Span, _ []byte) []entity.Span {
		for _, p := range preds[i] {
			t, _ := entity.Parse(p.Type)
			dst = append(dst, entity.Span{Start: p.Start, End: p.End, Type: t, Score: p.Score})
		}
		return dst
	})
}
