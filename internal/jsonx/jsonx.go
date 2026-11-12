package jsonx

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"sync"
)

const maxDepth = 64

var (
	ErrTooDeep         = errors.New("json nesting too deep")
	ErrTrailingData    = errors.New("trailing data after json value")
	ErrUnexpectedToken = errors.New("unexpected json token")
)

type Frame struct {
	Array bool
	Name  []byte
	Index int
}

func (f Frame) Is(name string) bool {
	return !f.Array && string(f.Name) == name
}

func IsString(raw []byte) bool {
	return len(raw) > 0 && raw[0] == '"'
}

func decodeName(raw []byte) ([]byte, error) {
	inner := raw[1 : len(raw)-1]
	if bytes.IndexByte(inner, '\\') < 0 {
		return inner, nil
	}
	out, err := jsontext.AppendUnquote(nil, raw)
	if err != nil {
		return nil, fmt.Errorf("decode name: %w", err)
	}
	return out, nil
}

type Visitor func(dst []byte, path []Frame, raw []byte) ([]byte, bool, error)

type walker struct {
	rd    bytes.Reader
	dec   *jsontext.Decoder
	stack []Frame
}

type Rewriter struct {
	pool sync.Pool
}

func NewRewriter() *Rewriter {
	return &Rewriter{}
}

func (r *Rewriter) get(doc []byte) *walker {
	w, ok := r.pool.Get().(*walker)
	if !ok {
		w = &walker{stack: make([]Frame, 0, 16)}
		w.dec = jsontext.NewDecoder(&w.rd)
	}
	w.rd.Reset(doc)
	w.dec.Reset(&w.rd)
	w.stack = w.stack[:0]
	return w
}

func (r *Rewriter) put(w *walker) {
	w.rd.Reset(nil)
	w.dec.Reset(&w.rd)
	for i := range w.stack {
		w.stack[i] = Frame{}
	}
	r.pool.Put(w)
}

func (r *Rewriter) Rewrite(dst, doc []byte, visit Visitor) ([]byte, error) {
	w := r.get(doc)
	defer r.put(w)
	base := len(dst)
	out, err := w.run(dst, doc, visit)
	if err != nil {
		return dst[:base], err
	}
	return out, nil
}

type pass struct {
	w     *walker
	doc   []byte
	dst   []byte
	tmp   []byte
	last  int
	visit Visitor
}

func (w *walker) run(dst, doc []byte, visit Visitor) ([]byte, error) {
	p := pass{w: w, doc: doc, dst: dst, visit: visit}
	started := false
	for !started || w.dec.StackDepth() > 0 {
		started = true
		if err := p.step(); err != nil {
			return p.dst, err
		}
	}
	if _, err := w.dec.ReadToken(); !errors.Is(err, io.EOF) {
		if err == nil {
			return p.dst, ErrTrailingData
		}
		return p.dst, fmt.Errorf("trailing data: %w", err)
	}
	return append(p.dst, doc[p.last:]...), nil
}

func (p *pass) atName(kind jsontext.Kind) bool {
	w := p.w
	top := len(w.stack) - 1
	if top < 0 || w.stack[top].Array || kind != '"' {
		return false
	}
	_, n := w.dec.StackIndex(w.dec.StackDepth())
	return n%2 == 0
}

func (p *pass) step() error {
	w := p.w
	kind := w.dec.PeekKind()
	top := len(w.stack) - 1
	if p.atName(kind) {
		raw, err := w.dec.ReadValue()
		if err != nil {
			return fmt.Errorf("read name: %w", err)
		}
		end := int(w.dec.InputOffset())
		name, err := decodeName(p.doc[end-len(raw) : end])
		if err != nil {
			return err
		}
		w.stack[top].Name = name
		return nil
	}
	if top >= 0 && w.stack[top].Array && kind != ']' {
		w.stack[top].Index++
	}
	switch kind {
	case jsontext.KindBeginObject, jsontext.KindBeginArray:
		if len(w.stack) >= maxDepth {
			return ErrTooDeep
		}
		if _, err := w.dec.ReadToken(); err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		w.stack = append(w.stack, Frame{Array: kind == jsontext.KindBeginArray, Index: -1})
	case jsontext.KindEndObject, jsontext.KindEndArray:
		if _, err := w.dec.ReadToken(); err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		w.stack = w.stack[:top]
	case jsontext.KindString, jsontext.KindNumber:
		return p.str()
	case jsontext.KindNull, jsontext.KindFalse, jsontext.KindTrue:
		if _, err := w.dec.ReadValue(); err != nil {
			return fmt.Errorf("read value: %w", err)
		}
	case jsontext.KindInvalid:
		if _, err := w.dec.ReadToken(); err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		return ErrUnexpectedToken
	}
	return nil
}

func (p *pass) str() error {
	w := p.w
	raw, err := w.dec.ReadValue()
	if err != nil {
		return fmt.Errorf("read string: %w", err)
	}
	end := int(w.dec.InputOffset())
	start := end - len(raw)
	out, handled, err := p.visit(p.tmp[:0], w.stack, p.doc[start:end])
	p.tmp = out
	if err != nil {
		return err
	}
	if handled {
		p.dst = append(append(p.dst, p.doc[p.last:start]...), out...)
		p.last = end
	}
	return nil
}

func Unquote(dst, raw []byte) ([]byte, error) {
	out, err := jsontext.AppendUnquote(dst, raw)
	if err != nil {
		return dst, fmt.Errorf("unquote: %w", err)
	}
	return out, nil
}

func Quote(dst, s []byte) []byte {
	out, err := jsontext.AppendQuote(dst, s)
	if err != nil {
		return appendQuoteLossy(dst, s)
	}
	return out
}

func appendQuoteLossy(dst, s []byte) []byte {
	clean := bytes.ToValidUTF8(s, []byte("�"))
	out, err := jsontext.AppendQuote(dst, clean)
	if err != nil {
		return append(dst, `""`...)
	}
	return out
}

func Valid(doc []byte) bool {
	return jsontext.Value(doc).IsValid()
}

const minBlob = 128

func LooksBase64(raw []byte) bool {
	if len(raw) < minBlob+2 || !IsString(raw) {
		return false
	}
	for _, c := range raw[1 : len(raw)-1] {
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '+' || c == '/' || c == '=' || c == '-' || c == '_' || c == '\\'
		if !ok {
			return false
		}
	}
	return true
}

func HasPrefix(raw []byte, prefix string) bool {
	return IsString(raw) && bytes.HasPrefix(raw[1:], []byte(prefix))
}

func Under(path []Frame, name string) bool {
	for _, f := range path {
		if f.Is(name) {
			return true
		}
	}
	return false
}
