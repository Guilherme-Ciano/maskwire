package restore

import (
	"bytes"
	"unicode/utf8"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const (
	MaxTokenLen = 32
	minSuffix   = 5
)

type Set interface {
	Lookup(t entity.Type, suffixUpper []byte) ([]byte, bool)
}

type Foreign uint8

const (
	ForeignGeneric Foreign = iota
	ForeignKeep
)

type Encoding uint8

const (
	Raw Encoding = iota
	JSONString
)

type Options struct {
	Foreign  Foreign
	Encoding Encoding
}

type Stats struct {
	Restored     uint32
	Foreign      uint32
	ForeignTypes [entity.Count]uint32
	MaxHeld      int
}

func (s *Stats) Add(o *Stats) {
	s.Restored += o.Restored
	s.Foreign += o.Foreign
	for i := range s.ForeignTypes {
		s.ForeignTypes[i] += o.ForeignTypes[i]
	}
	s.MaxHeld = max(s.MaxHeld, o.MaxHeld)
}

type result uint8

const (
	reject result = iota
	partial
	full
)

type match struct {
	res    result
	n      int
	typ    entity.Type
	suffix [token.MaxLength]byte
	slen   int
}

func letter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

type matcher struct {
	b       []byte
	i       int
	escapes bool
	m       match
}

func (x *matcher) at(k int) (byte, bool) {
	if k < len(x.b) {
		return x.b[k], true
	}
	return 0, false
}

func (x *matcher) expect(want byte) result {
	c, ok := x.at(x.i)
	if !ok {
		return partial
	}
	if c == want {
		x.i++
		return full
	}
	if !x.escapes || c != '\\' {
		return reject
	}
	c2, ok := x.at(x.i + 1)
	if !ok {
		return partial
	}
	if c2 == want {
		x.i += 2
		return full
	}
	return reject
}

func (x *matcher) optionalSpace() result {
	c, ok := x.at(x.i)
	if !ok {
		return partial
	}
	if c == ' ' {
		x.i++
	}
	return full
}

func (x *matcher) label() result {
	start := x.i
	for {
		c, ok := x.at(x.i)
		if !ok {
			return partial
		}
		if !letter(c) {
			break
		}
		x.i++
		if x.i-start > entity.MaxLabelLen {
			return reject
		}
	}
	t, ok := entity.LabelFold(x.b[start:x.i])
	if !ok {
		return reject
	}
	x.m.typ = t
	return full
}

func (x *matcher) suffix() result {
	for {
		c, ok := x.at(x.i)
		if !ok {
			return partial
		}
		u, isCrock := token.CrockfordFold(c)
		if !isCrock {
			break
		}
		if x.m.slen == len(x.m.suffix) {
			return reject
		}
		x.m.suffix[x.m.slen] = u
		x.m.slen++
		x.i++
	}
	if x.m.slen < minSuffix {
		return reject
	}
	if c, _ := x.at(x.i); c == ' ' {
		x.i++
	}
	return full
}

func (x *matcher) done(r result) match {
	x.m.res = r
	if r == full {
		x.m.n = x.i
	}
	return x.m
}

func matchToken(b []byte, escapes bool) match {
	x := matcher{b: b, escapes: escapes}
	if r := x.expect('['); r != full {
		return x.done(r)
	}
	if r := x.optionalSpace(); r != full {
		return x.done(r)
	}
	if r := x.label(); r != full {
		return x.done(r)
	}
	if r := x.expect('_'); r != full {
		return x.done(r)
	}
	if r := x.suffix(); r != full {
		return x.done(r)
	}
	return x.done(x.expect(']'))
}

type Stream struct {
	set     Set
	opts    Options
	pending [MaxTokenLen]byte
	held    int
	key     [token.MaxLength]byte
	stats   Stats
}

func NewStream(set Set, o Options) Stream {
	return Stream{set: set, opts: o}
}

func (s *Stream) Held() int {
	return s.held
}

func (s *Stream) Stats() *Stats {
	return &s.stats
}

func (s *Stream) nextOpener(b []byte) int {
	if s.opts.Encoding == Raw {
		return bytes.IndexAny(b, "[\\")
	}
	return bytes.IndexByte(b, '[')
}

func (s *Stream) process(dst, b []byte, limit int, final bool) (out []byte, next, partialAt int) {
	i := 0
	for i < limit {
		j := s.nextOpener(b[i:limit])
		if j < 0 {
			return append(dst, b[i:limit]...), limit, -1
		}
		j += i
		dst = append(dst, b[i:j]...)
		m := matchToken(b[j:], s.opts.Encoding == Raw)
		switch m.res {
		case full:
			dst = s.resolve(dst, b[j:j+m.n], &m)
			i = j + m.n
		case partial:
			if final {
				return append(dst, b[j:]...), len(b), -1
			}
			return dst, j, j
		case reject:
			dst = append(dst, b[j])
			i = j + 1
		}
	}
	return dst, i, -1
}

func (s *Stream) Feed(dst, chunk []byte) []byte {
	if s.held > 0 {
		var win [2 * MaxTokenLen]byte
		old := s.held
		take := min(len(chunk), MaxTokenLen)
		w := append(append(win[:0], s.pending[:old]...), chunk[:take]...)
		s.held = 0
		var next, part int
		dst, next, part = s.process(dst, w, old, false)
		if part >= 0 {
			s.hold(w[part:])
			return dst
		}
		chunk = chunk[next-old:]
	}
	dst, _, part := s.process(dst, chunk, len(chunk), false)
	if part >= 0 {
		s.hold(chunk[part:])
	}
	return dst
}

func (s *Stream) hold(b []byte) {
	s.held = copy(s.pending[:], b)
	s.stats.MaxHeld = max(s.stats.MaxHeld, s.held)
}

func (s *Stream) Flush(dst []byte) []byte {
	dst = append(dst, s.pending[:s.held]...)
	s.held = 0
	return dst
}

func (s *Stream) resolve(dst, raw []byte, m *match) []byte {
	n := copy(s.key[:], m.suffix[:m.slen])
	if orig, ok := s.set.Lookup(m.typ, s.key[:n]); ok {
		s.stats.Restored++
		if s.opts.Encoding == JSONString {
			return AppendJSONContent(dst, orig)
		}
		return append(dst, orig...)
	}
	s.stats.Foreign++
	s.stats.ForeignTypes[m.typ]++
	if s.opts.Foreign == ForeignKeep {
		return append(dst, raw...)
	}
	dst = append(dst, '[')
	dst = append(dst, m.typ.LabelLower()...)
	return append(dst, ']')
}

func Restore(dst, src []byte, set Set, o Options) ([]byte, Stats) {
	s := NewStream(set, o)
	dst, _, _ = s.process(dst, src, len(src), true)
	return dst, s.stats
}

func AppendJSONContent(dst, s []byte) []byte {
	const hex = "0123456789abcdef"
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '"' || c == '\\':
				dst = append(dst, '\\', c)
			case c == '\n':
				dst = append(dst, '\\', 'n')
			case c == '\r':
				dst = append(dst, '\\', 'r')
			case c == '\t':
				dst = append(dst, '\\', 't')
			case c < 0x20:
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			default:
				dst = append(dst, c)
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			dst = append(dst, `�`...)
		case r == ' ':
			dst = append(dst, ` `...)
		case r == ' ':
			dst = append(dst, ` `...)
		default:
			dst = append(dst, s[i:i+n]...)
		}
		i += n
	}
	return dst
}
