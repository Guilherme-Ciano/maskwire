package openai

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strconv"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

const (
	maxFastChoices = 4
	jsonNull       = "null"
)

type span struct {
	start int
	end   int
	ok    bool
}

type fastChoice struct {
	index      int
	hasDelta   bool
	other      bool
	deltaOther bool
	text       [2]span
}

type fastChunk struct {
	n       int
	payload bool
	choices [maxFastChoices]fastChoice
}

type chunkParser struct {
	rd  bytes.Reader
	dec *jsontext.Decoder
	c   fastChunk
}

func newChunkParser() *chunkParser {
	p := &chunkParser{}
	p.dec = jsontext.NewDecoder(&p.rd)
	return p
}

func (p *chunkParser) parse(data []byte) bool {
	p.rd.Reset(data)
	p.dec.Reset(&p.rd)
	p.c = fastChunk{}
	if !p.open('{') {
		return false
	}
	for p.dec.PeekKind() != '}' {
		name, ok := p.name()
		if !ok {
			return false
		}
		switch string(name) {
		case `"choices"`:
			if !p.choices() {
				return false
			}
		case `"usage"`:
			v, err := p.dec.ReadValue()
			if err != nil {
				return false
			}
			p.c.payload = string(v) != jsonNull
		default:
			if p.dec.SkipValue() != nil {
				return false
			}
		}
	}
	if !p.open('}') {
		return false
	}
	_, err := p.dec.ReadToken()
	return errors.Is(err, io.EOF)
}

func (p *chunkParser) open(kind jsontext.Kind) bool {
	tok, err := p.dec.ReadToken()
	return err == nil && tok.Kind() == kind
}

func (p *chunkParser) name() ([]byte, bool) {
	v, err := p.dec.ReadValue()
	if err != nil || bytes.IndexByte(v, '\\') >= 0 {
		return nil, false
	}
	return v, true
}

func (p *chunkParser) choices() bool {
	if p.dec.PeekKind() == 'n' {
		return p.dec.SkipValue() == nil
	}
	if !p.open('[') {
		return false
	}
	for p.dec.PeekKind() != ']' {
		if p.c.n == maxFastChoices || !p.choice(&p.c.choices[p.c.n]) {
			return false
		}
		p.c.n++
	}
	return p.open(']')
}

func (p *chunkParser) choice(ch *fastChoice) bool {
	if !p.open('{') {
		return false
	}
	for p.dec.PeekKind() != '}' {
		name, ok := p.name()
		if !ok || !p.choiceField(ch, name) {
			return false
		}
	}
	return p.open('}')
}

func (p *chunkParser) choiceField(ch *fastChoice, name []byte) bool {
	switch string(name) {
	case `"index"`:
		v, err := p.dec.ReadValue()
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(string(v))
		ch.index = n
		return err == nil
	case `"finish_reason"`:
		v, err := p.dec.ReadValue()
		return err == nil && string(v) == jsonNull
	case `"delta"`:
		return p.delta(ch)
	}
	ch.other = true
	return p.dec.SkipValue() == nil
}

func (p *chunkParser) delta(ch *fastChoice) bool {
	if p.dec.PeekKind() == 'n' {
		return p.dec.SkipValue() == nil
	}
	if !p.open('{') {
		return false
	}
	ch.hasDelta = true
	for p.dec.PeekKind() != '}' {
		name, ok := p.name()
		if !ok {
			return false
		}
		switch string(name) {
		case `"content"`:
			if !p.text(&ch.text[kindContent]) {
				return false
			}
		case `"refusal"`:
			if !p.text(&ch.text[kindRefusal]) {
				return false
			}
		case `"tool_calls"`:
			return false
		default:
			if p.dec.SkipValue() != nil {
				return false
			}
			ch.deltaOther = true
		}
	}
	return p.open('}')
}

func (p *chunkParser) text(s *span) bool {
	kind := p.dec.PeekKind()
	v, err := p.dec.ReadValue()
	if err != nil {
		return false
	}
	if kind == '"' {
		end := int(p.dec.InputOffset())
		*s = span{start: end - len(v), end: end, ok: true}
		return true
	}
	return kind == 'n'
}

func (b *StreamBody) reserveFast(c *fastChunk) error {
	need := 0
	for i := range c.n {
		ch := &c.choices[i]
		if !ch.hasDelta {
			continue
		}
		for kind := kindContent; kind <= kindRefusal; kind++ {
			if !b.known(channelKey{ch.index, kind, 0}) {
				need++
			}
		}
	}
	if len(b.channels)+need > codec.MaxChannels {
		return codec.ErrTooManyChannels
	}
	return nil
}

type splice struct {
	at   span
	from int
	to   int
}

type edits struct {
	list [2 * maxFastChoices]splice
	n    int
}

func (b *StreamBody) rewriteFast(data []byte, c *fastChunk) (encoded []byte, keep bool, err error) {
	if c.n == 0 {
		return data, true, nil
	}
	b.last = append(b.last[:0], data...)
	if err := b.reserveFast(c); err != nil {
		return nil, false, err
	}
	var ed edits
	b.scratch = b.scratch[:0]
	empty := true
	for i := range c.n {
		blank, err := b.feedFastChoice(data, &c.choices[i], &ed)
		if err != nil {
			return nil, false, err
		}
		empty = empty && blank
	}
	if empty && !c.payload {
		return nil, false, nil
	}
	if ed.n == 0 {
		return data, true, nil
	}
	return b.applySplices(data, ed.list[:ed.n]), true, nil
}

func (b *StreamBody) feedFastChoice(data []byte, ch *fastChoice, ed *edits) (blank bool, err error) {
	blank = !ch.other && !ch.deltaOther
	for kind := kindContent; kind <= kindRefusal; kind++ {
		s := ch.text[kind]
		if !s.ok {
			continue
		}
		from := len(b.scratch)
		changed, err := b.feedRaw(channelKey{ch.index, kind, 0}, data[s.start:s.end])
		if err != nil {
			return false, err
		}
		if len(b.scratch) > from {
			blank = false
		}
		if !changed {
			b.scratch = b.scratch[:from]
			continue
		}
		ed.list[ed.n] = splice{at: s, from: from, to: len(b.scratch)}
		ed.n++
	}
	return blank, nil
}

func (b *StreamBody) feedRaw(k channelKey, raw []byte) (bool, error) {
	st, err := b.channel(k)
	if err != nil {
		return false, err
	}
	b.plain, err = jsonx.Unquote(b.plain[:0], raw)
	if err != nil {
		return false, err
	}
	from := len(b.scratch)
	b.scratch = st.Feed(b.scratch, b.plain)
	return !bytes.Equal(b.scratch[from:], b.plain), nil
}

func (b *StreamBody) applySplices(data []byte, edits []splice) []byte {
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j].at.start < edits[j-1].at.start; j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
	out := b.enc[:0]
	last := 0
	for _, e := range edits {
		out = append(out, data[last:e.at.start]...)
		out = jsonx.Quote(out, b.scratch[e.from:e.to])
		last = e.at.end
	}
	out = append(out, data[last:]...)
	b.enc = out
	return out
}
