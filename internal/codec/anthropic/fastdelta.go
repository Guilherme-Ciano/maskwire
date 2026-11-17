package anthropic

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strconv"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

type textSpan struct {
	start int
	end   int
	ok    bool
}

type fastDelta struct {
	index     int
	hasDelta  bool
	deltaType blockKind
	text      textSpan
	json      textSpan
}

type deltaParser struct {
	rd  bytes.Reader
	dec *jsontext.Decoder
	d   fastDelta
}

func newDeltaParser() *deltaParser {
	p := &deltaParser{}
	p.dec = jsontext.NewDecoder(&p.rd)
	return p
}

func (p *deltaParser) parse(data []byte) bool {
	p.rd.Reset(data)
	p.dec.Reset(&p.rd)
	p.d = fastDelta{}
	isDelta := false
	if !p.open('{') {
		return false
	}
	for p.dec.PeekKind() != '}' {
		name, ok := p.name()
		if !ok {
			return false
		}
		switch string(name) {
		case `"type"`:
			v, err := p.dec.ReadValue()
			if err != nil {
				return false
			}
			isDelta = string(v) == `"`+evBlockDelta+`"`
		case `"index"`:
			if !p.index() {
				return false
			}
		case `"delta"`:
			if !p.delta() {
				return false
			}
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
	return isDelta && errors.Is(err, io.EOF)
}

func (p *deltaParser) open(kind jsontext.Kind) bool {
	tok, err := p.dec.ReadToken()
	return err == nil && tok.Kind() == kind
}

func (p *deltaParser) name() ([]byte, bool) {
	v, err := p.dec.ReadValue()
	if err != nil || bytes.IndexByte(v, '\\') >= 0 {
		return nil, false
	}
	return v, true
}

func (p *deltaParser) index() bool {
	v, err := p.dec.ReadValue()
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(string(v))
	p.d.index = n
	return err == nil
}

func (p *deltaParser) delta() bool {
	if p.dec.PeekKind() == 'n' {
		return p.dec.SkipValue() == nil
	}
	if !p.open('{') {
		return false
	}
	p.d.hasDelta = true
	for p.dec.PeekKind() != '}' {
		name, ok := p.name()
		if !ok || !p.deltaField(name) {
			return false
		}
	}
	return p.open('}')
}

func (p *deltaParser) deltaField(name []byte) bool {
	switch string(name) {
	case `"type"`:
		v, err := p.dec.ReadValue()
		if err != nil || bytes.IndexByte(v, '\\') >= 0 {
			return false
		}
		switch string(v) {
		case `"` + deltaText + `"`:
			p.d.deltaType = kindText
		case `"` + deltaJSON + `"`:
			p.d.deltaType = kindJSON
		default:
			p.d.deltaType = kindPassthrough
		}
		return true
	case `"text"`:
		return p.str(&p.d.text)
	case `"partial_json"`:
		return p.str(&p.d.json)
	}
	return p.dec.SkipValue() == nil
}

func (p *deltaParser) str(s *textSpan) bool {
	kind := p.dec.PeekKind()
	v, err := p.dec.ReadValue()
	if err != nil {
		return false
	}
	if kind == '"' {
		end := int(p.dec.InputOffset())
		*s = textSpan{start: end - len(v), end: end, ok: true}
		return true
	}
	return kind == 'n'
}

func (b *StreamBody) fastDelta(data []byte, d *fastDelta) (out []byte, keep bool, err error) {
	c := b.find(d.index)
	if c == nil || c.kind == kindPassthrough || !d.hasDelta {
		return nil, true, nil
	}
	var field textSpan
	switch {
	case c.kind == kindText && d.deltaType == kindText:
		field = d.text
	case c.kind == kindJSON && d.deltaType == kindJSON:
		field = d.json
	}
	if !field.ok {
		b.out = b.flush(b.out, c)
		return nil, true, nil
	}
	b.plain, err = jsonx.Unquote(b.plain[:0], data[field.start:field.end])
	if err != nil {
		return nil, false, err
	}
	b.scratch = c.stream.Feed(b.scratch[:0], b.plain)
	if len(b.scratch) == 0 {
		return nil, false, nil
	}
	if bytes.Equal(b.scratch, b.plain) {
		return nil, true, nil
	}
	enc := b.enc[:0]
	enc = append(enc, data[:field.start]...)
	enc = jsonx.Quote(enc, b.scratch)
	enc = append(enc, data[field.end:]...)
	b.enc = enc
	return enc, true, nil
}
