package restore

import (
	"bytes"
	"unicode/utf8"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const (
	MaxTokenLen = 48
	minSuffix   = 5
)

type Set interface {
	Lookup(t entity.Type, suffixUpper []byte) ([]byte, bool)
	Numeric(suffixUpper []byte) bool
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
	Tap      func(upstream []byte)
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

type opener uint8

const (
	openNone opener = iota
	openBracket
	openEscBracket
	openAngle
	openParen
	openBrace
	openLenticular
	openTortoise
	openPercent
	openUnicode
)

const (
	maxRelaxedLabel = 12
	maxSeparators   = 2
)

type match struct {
	res       result
	n         int
	typ       entity.Type
	canonical bool
	suffix    [token.MaxLength]byte
	slen      int
}

func letter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func wordByte(c byte) bool {
	return letter(c) || (c >= '0' && c <= '9') || c == '_'
}

func closerFor(o opener) string {
	switch o {
	case openBracket, openEscBracket:
		return "]"
	case openAngle:
		return ">"
	case openParen:
		return ")"
	case openBrace:
		return "}"
	case openLenticular:
		return "\xe3\x80\x91"
	case openTortoise:
		return "\xe3\x80\x95"
	case openPercent:
		return "%5d"
	case openUnicode:
		return "\\u005d"
	case openNone:
		return ""
	}
	return ""
}

func aliasType(label []byte) (entity.Type, bool) {
	var buf [maxRelaxedLabel]byte
	k := 0
	for _, c := range label {
		if c == '-' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		buf[k] = c
		k++
	}
	if t, ok := entity.LabelFold(buf[:k]); ok {
		return t, true
	}
	switch string(buf[:k]) {
	case "MAIL", "CORREO", "COURRIEL", "CORREIO":
		return entity.Email, true
	case "TEL", "TELEFONE", "TELEFONO", "TELEPHONE", "TELEFON", "FONE", "MOBILE", "CELULAR":
		return entity.Phone, true
	}
	return entity.Unknown, false
}

type matcher struct {
	b    []byte
	i    int
	prev byte
	json bool
	eof  bool
	open opener
	m    match
}

func (x *matcher) at(k int) (byte, bool) {
	if k < len(x.b) {
		return x.b[k], true
	}
	return 0, false
}

func (x *matcher) seq(want string) result {
	for k := range len(want) {
		c, ok := x.at(x.i + k)
		if !ok {
			if x.eof {
				return reject
			}
			return partial
		}
		w := want[k]
		if c >= 'A' && c <= 'Z' && w >= 'a' && w <= 'z' {
			c += 'a' - 'A'
		}
		if c != w {
			return reject
		}
	}
	x.i += len(want)
	return full
}

func (x *matcher) opening() result {
	c, ok := x.at(x.i)
	if !ok {
		return x.more()
	}
	switch {
	case c == '[':
		x.open, x.i = openBracket, x.i+1
		return full
	case c == '<':
		x.open, x.i = openAngle, x.i+1
		return full
	case c == '(':
		x.open, x.i = openParen, x.i+1
		return full
	case c == '{':
		x.open, x.i = openBrace, x.i+1
		return full
	case c == '%':
		x.open = openPercent
		return x.seq("%5b")
	case c == 0xE3:
		return x.cjkOpening()
	case c == '\\' && x.json:
		x.open = openUnicode
		return x.seq("\\u005b")
	case c == '\\':
		x.open = openEscBracket
		return x.seq("\\[")
	case c >= 'A' && c <= 'Z' && !wordByte(x.prev):
		x.open = openNone
		return full
	}
	return reject
}

func (x *matcher) cjkOpening() result {
	if r := x.seq("\xe3\x80"); r != full {
		return r
	}
	c, ok := x.at(x.i)
	if !ok {
		if x.eof {
			return reject
		}
		return partial
	}
	switch c {
	case 0x90:
		x.open = openLenticular
	case 0x94:
		x.open = openTortoise
	default:
		return reject
	}
	x.i++
	return full
}

func (x *matcher) more() result {
	if x.eof {
		return reject
	}
	return partial
}

func (x *matcher) space() {
	if c, ok := x.at(x.i); ok && c == ' ' && x.open != openNone {
		x.i++
	}
}

func (x *matcher) label() (r result, lbl []byte) {
	start := x.i
	for {
		c, ok := x.at(x.i)
		if !ok {
			return x.more(), nil
		}
		isLabel := letter(c) || (c == '-' && x.i > start)
		if x.open == openNone && c >= 'a' && c <= 'z' {
			isLabel = false
		}
		if !isLabel {
			break
		}
		x.i++
		if x.i-start > maxRelaxedLabel {
			return reject, nil
		}
	}
	if x.i == start {
		return reject, nil
	}
	return full, x.b[start:x.i]
}

func (x *matcher) separator() (result, bool) {
	start := x.i
	plain := false
	for range maxSeparators {
		c, ok := x.at(x.i)
		if !ok {
			return x.more(), false
		}
		switch {
		case c == '_' || c == '-' || c == ' ':
			x.i++
		case c == '\\' && !x.json:
			c2, ok2 := x.at(x.i + 1)
			if !ok2 {
				return x.more(), false
			}
			if c2 != '_' {
				return x.sepDone(start, plain)
			}
			x.i += 2
		default:
			return x.sepDone(start, plain)
		}
		plain = x.i-start == 1 && x.b[start] == '_' || x.i-start == 2 && x.b[start] == '\\'
	}
	return x.sepDone(start, plain)
}

func (x *matcher) sepDone(start int, plain bool) (result, bool) {
	if x.i == start {
		return reject, false
	}
	return full, plain
}

func (x *matcher) suffix() result {
	for {
		c, ok := x.at(x.i)
		if !ok {
			if x.eof {
				break
			}
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
	return full
}

func (x *matcher) boundaryAfter() result {
	c, ok := x.at(x.i)
	if !ok {
		if x.eof {
			return full
		}
		return partial
	}
	if wordByte(c) {
		return reject
	}
	return full
}

func (x *matcher) closing() (result, bool) {
	if x.open == openNone {
		return x.boundaryAfter(), false
	}
	beforeSpace := x.i
	x.space()
	want := closerFor(x.open)
	r := x.seq(want)
	if r == full {
		return full, true
	}
	if x.open == openEscBracket && r == reject {
		if r2 := x.seq("\\]"); r2 != reject {
			return r2, r2 == full
		}
	}
	if r == partial {
		return partial, false
	}
	x.i = beforeSpace
	return x.boundaryAfter(), false
}

func (x *matcher) done(r result) match {
	x.m.res = r
	if r == full {
		x.m.n = x.i
	}
	return x.m
}

func matchToken(b []byte, prev byte, json, eof bool) match {
	x := matcher{b: b, prev: prev, json: json, eof: eof}
	if r := x.opening(); r != full {
		return x.done(r)
	}
	x.space()
	r, lbl := x.label()
	if r != full {
		return x.done(r)
	}
	r, plainSep := x.separator()
	if r == reject {
		if dash := bytes.LastIndexByte(lbl, '-'); dash > 0 {
			x.i -= len(lbl) - dash
			lbl = lbl[:dash]
			r, plainSep = x.separator()
		}
	}
	if r != full {
		return x.done(r)
	}
	if r = x.suffix(); r != full {
		return x.done(r)
	}
	r, closed := x.closing()
	if r != full {
		return x.done(r)
	}
	t, known := entity.LabelFold(lbl)
	bracket := x.open == openBracket || x.open == openEscBracket
	x.m.canonical = known && bracket && closed && plainSep
	if !x.m.canonical {
		t, _ = aliasType(lbl)
	}
	x.m.typ = t
	return x.done(full)
}

type Stream struct {
	set     Set
	opts    Options
	pending [MaxTokenLen]byte
	held    int
	prev    byte
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

func (s *Stream) trigger(c byte) bool {
	if c == '"' {
		return s.opts.Encoding == JSONString
	}
	switch c {
	case '[', '<', '(', '{', '%', '\\', 0xE3:
		return true
	}
	return c >= 'A' && c <= 'Z'
}

func (s *Stream) nextTrigger(b []byte) int {
	for k, c := range b {
		if c >= 'a' && c <= 'z' {
			continue
		}
		if s.trigger(c) {
			return k
		}
	}
	return -1
}

func bareCandidate(b []byte, j int, prev byte) bool {
	if wordByte(before(b, j, prev)) {
		return false
	}
	if j+1 < len(b) {
		n := b[j+1]
		return (n >= 'A' && n <= 'Z') || n == '-'
	}
	return true
}

func before(b []byte, j int, prev byte) byte {
	if j > 0 {
		return b[j-1]
	}
	return prev
}

func (s *Stream) process(dst, b []byte, limit int, final bool, prev byte) (out []byte, next, partialAt int) {
	i := 0
	for i < limit {
		j := s.nextTrigger(b[i:limit])
		if j < 0 {
			return append(dst, b[i:limit]...), limit, -1
		}
		j += i
		dst = append(dst, b[i:j]...)
		if c := b[j]; c >= 'A' && c <= 'Z' && !bareCandidate(b, j, prev) {
			dst = append(dst, c)
			i = j + 1
			continue
		}
		if b[j] == '"' {
			n, out, res := s.quotedNumber(dst, b[j:], final)
			switch res {
			case partial:
				return dst, j, j
			case full:
				dst = out
				i = j + n
			case reject:
				dst = append(dst, '"')
				i = j + 1
			}
			continue
		}
		m := matchToken(b[j:], before(b, j, prev), s.opts.Encoding == JSONString, final)
		switch m.res {
		case full:
			if out, ok := s.resolve(dst, b[j:j+m.n], &m); ok {
				dst = out
				i = j + m.n
				continue
			}
			dst = append(dst, b[j])
			i = j + 1
		case partial:
			return dst, j, j
		case reject:
			dst = append(dst, b[j])
			i = j + 1
		}
	}
	return dst, i, -1
}

func (s *Stream) quotedNumber(dst, b []byte, final bool) (n int, out []byte, res result) {
	if len(b) < 2 {
		if final {
			return 0, dst, reject
		}
		return 0, dst, partial
	}
	m := matchToken(b[1:], '"', true, final)
	switch m.res {
	case partial:
		return 0, dst, partial
	case reject:
		return 0, dst, reject
	case full:
	}
	end := 1 + m.n
	if end >= len(b) {
		if final {
			return 0, dst, reject
		}
		return 0, dst, partial
	}
	if b[end] != '"' {
		return 0, dst, reject
	}
	k := copy(s.key[:], m.suffix[:m.slen])
	if !s.set.Numeric(s.key[:k]) {
		return 0, dst, reject
	}
	orig, ok := s.set.Lookup(m.typ, s.key[:k])
	if !ok {
		return 0, dst, reject
	}
	s.stats.Restored++
	return end + 1, append(dst, orig...), full
}

func (s *Stream) Feed(dst, chunk []byte) []byte {
	if len(chunk) == 0 {
		return dst
	}
	if s.opts.Tap != nil {
		s.opts.Tap(chunk)
	}
	if s.held > 0 {
		var win [2 * MaxTokenLen]byte
		old := s.held
		take := min(len(chunk), MaxTokenLen)
		w := append(append(win[:0], s.pending[:old]...), chunk[:take]...)
		s.held = 0
		var next, part int
		dst, next, part = s.process(dst, w, old, false, s.prev)
		if part >= 0 {
			s.hold(w, part)
			return dst
		}
		s.prev = w[next-1]
		chunk = chunk[next-old:]
		if len(chunk) == 0 {
			return dst
		}
	}
	dst, _, part := s.process(dst, chunk, len(chunk), false, s.prev)
	if part >= 0 {
		s.hold(chunk, part)
		return dst
	}
	s.prev = chunk[len(chunk)-1]
	return dst
}

func (s *Stream) hold(b []byte, at int) {
	s.prev = before(b, at, s.prev)
	s.held = copy(s.pending[:], b[at:])
	s.stats.MaxHeld = max(s.stats.MaxHeld, s.held)
}

func (s *Stream) Flush(dst []byte) []byte {
	if s.held == 0 {
		return dst
	}
	var buf [MaxTokenLen]byte
	n := copy(buf[:], s.pending[:s.held])
	s.held = 0
	dst, _, _ = s.process(dst, buf[:n], n, true, s.prev)
	s.prev = buf[n-1]
	return dst
}

func (s *Stream) resolve(dst, raw []byte, m *match) ([]byte, bool) {
	n := copy(s.key[:], m.suffix[:m.slen])
	if orig, ok := s.set.Lookup(m.typ, s.key[:n]); ok {
		s.stats.Restored++
		if s.opts.Encoding == JSONString {
			return AppendJSONContent(dst, orig), true
		}
		return append(dst, orig...), true
	}
	if !m.canonical {
		return dst, false
	}
	s.stats.Foreign++
	s.stats.ForeignTypes[m.typ]++
	if s.opts.Foreign == ForeignKeep {
		return append(dst, raw...), true
	}
	dst = append(dst, '[')
	dst = append(dst, m.typ.LabelLower()...)
	return append(dst, ']'), true
}

func Restore(dst, src []byte, set Set, o Options) ([]byte, Stats) {
	if o.Tap != nil {
		o.Tap(src)
	}
	s := NewStream(set, o)
	dst, _, _ = s.process(dst, src, len(src), true, 0)
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
