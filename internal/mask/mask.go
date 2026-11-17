package mask

import (
	"bytes"
	"errors"
	"strings"
	"sync"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const (
	DefaultMaxEntities = 4096
	suffixCap          = token.MaxLength
)

var (
	ErrTooManyEntities = errors.New("too many entities in request")
	ErrSuffixExhausted = errors.New("token suffix collision could not be resolved")
)

type BlockedError struct {
	Types []entity.Type
}

func (e *BlockedError) Error() string {
	names := make([]string, len(e.Types))
	for i, t := range e.Types {
		names[i] = t.String()
	}
	return "request contains blocked entity types: " + strings.Join(names, ",")
}

type SpanDetector interface {
	DetectWith(dst []entity.Span, text []byte, phones *detect.PhoneState) []entity.Span
	NewPhoneState() *detect.PhoneState
}

type Tenant struct {
	ID          string
	Key         *token.TenantKey
	Policy      *policy.Policy
	TokenLength int
}

type Engine struct {
	det         SpanDetector
	maxEntities int
	rw          *jsonx.Rewriter
	pool        sync.Pool
}

func NewEngine(det SpanDetector, maxEntities int) *Engine {
	if maxEntities <= 0 {
		maxEntities = DefaultMaxEntities
	}
	return &Engine{det: det, maxEntities: maxEntities, rw: jsonx.NewRewriter()}
}

type Counters struct {
	Detected [entity.Count]uint32
	Actions  [entity.Count][4]uint32
}

func (c *Counters) TotalDetected() uint32 {
	var n uint32
	for _, v := range c.Detected {
		n += v
	}
	return n
}

func (c *Counters) TotalAction(a policy.Action) uint32 {
	var n uint32
	for t := range c.Actions {
		n += c.Actions[t][a]
	}
	return n
}

type Suffix [suffixCap]byte

type entry struct {
	typ      entity.Type
	sum      [32]byte
	origFrom int
	origTo   int
	numeric  bool
}

type Session struct {
	eng      *Engine
	tenant   *Tenant
	key      *token.TenantKey
	observe  bool
	set      map[Suffix]int
	entries  []entry
	arena    []byte
	spans    []entity.Span
	canon    []byte
	scratch  []byte
	blocked  [entity.Count]bool
	counters Counters
	variants int
	phones   *detect.PhoneState
	last     int
}

func (s *Session) SurfaceVariants() int {
	return s.variants
}

func (e *Engine) Scan(text []byte) (counts [entity.Count]uint32) {
	spans := e.det.DetectWith(nil, text, e.det.NewPhoneState())
	for _, sp := range spans {
		counts[sp.Type]++
	}
	return counts
}

func (e *Engine) NewSession(t *Tenant) *Session {
	s, ok := e.pool.Get().(*Session)
	if !ok {
		s = &Session{eng: e, set: make(map[Suffix]int)}
	}
	s.tenant = t
	s.key = t.Key
	s.observe = t.Policy.Mode == policy.Observe
	s.phones = e.det.NewPhoneState()
	return s
}

func (s *Session) UseKey(k *token.TenantKey) {
	s.key = k
}

func (s *Session) Key() *token.TenantKey {
	return s.key
}

func (s *Session) Each(fn func(suffix []byte, t entity.Type, orig []byte, numeric bool)) {
	for key, idx := range s.set {
		e := s.entries[idx]
		n := 0
		for n < len(key) && key[n] != 0 {
			n++
		}
		fn(key[:n], e.typ, s.arena[e.origFrom:e.origTo], e.numeric)
	}
}

func (s *Session) Counters() *Counters {
	return &s.counters
}

func (s *Session) Issued() int {
	return len(s.entries)
}

func (s *Session) Observe() bool {
	return s.observe
}

func (s *Session) Err() error {
	var types []entity.Type
	for t, b := range s.blocked {
		if b {
			types = append(types, entity.Type(t))
		}
	}
	if len(types) == 0 {
		return nil
	}
	return &BlockedError{Types: types}
}

func (s *Session) MaskText(dst, src []byte) ([]byte, error) {
	s.spans = s.eng.det.DetectWith(s.spans[:0], src, s.phones)
	last := 0
	for _, sp := range s.spans {
		if sp.Start < last || sp.End > len(src) {
			continue
		}
		action := s.tenant.Policy.Decide(sp.Type, sp.Score)
		s.counters.Detected[sp.Type]++
		s.counters.Actions[sp.Type][action]++
		if s.observe {
			continue
		}
		switch action {
		case policy.Allow:
			continue
		case policy.Block:
			s.blocked[sp.Type] = true
			continue
		case policy.Redact:
			dst = append(dst, src[last:sp.Start]...)
			dst = token.AppendRedacted(dst, sp.Type)
		case policy.Mask:
			dst = append(dst, src[last:sp.Start]...)
			var err error
			dst, err = s.issue(dst, sp.Type, src[sp.Start:sp.End])
			if err != nil {
				return dst, err
			}
		}
		last = sp.End
	}
	return append(dst, src[last:]...), nil
}

func (s *Session) issue(dst []byte, t entity.Type, raw []byte) ([]byte, error) {
	s.canon = token.AppendCanonical(s.canon[:0], t, raw)
	sum := s.key.Sum(t, s.canon)
	n := s.tenant.TokenLength
	if n <= 0 {
		n = token.DefaultLength
	}
	n = max(n, token.MinLength)
	for ; n <= suffixCap; n += 2 {
		var key Suffix
		token.AppendSuffix(key[:0], &sum, n)
		idx, ok := s.set[key]
		if ok && s.entries[idx].sum != sum {
			continue
		}
		if ok && !bytes.Equal(s.arena[s.entries[idx].origFrom:s.entries[idx].origTo], raw) {
			s.variants++
		}
		s.last = idx
		if !ok {
			if len(s.entries) >= s.eng.maxEntities {
				return dst, ErrTooManyEntities
			}
			from := len(s.arena)
			s.arena = append(s.arena, raw...)
			s.entries = append(s.entries, entry{typ: t, sum: sum, origFrom: from, origTo: len(s.arena)})
			s.set[key] = len(s.entries) - 1
			s.last = len(s.entries) - 1
		}
		return token.AppendToken(dst, t, key[:n]), nil
	}
	return dst, ErrSuffixExhausted
}

func (s *Session) Lookup(t entity.Type, suffixUpper []byte) ([]byte, bool) {
	if len(suffixUpper) > suffixCap {
		return nil, false
	}
	var key Suffix
	copy(key[:], suffixUpper)
	idx, ok := s.set[key]
	if !ok {
		return nil, false
	}
	e := s.entries[idx]
	if t != entity.Unknown && e.typ.Label() != t.Label() {
		return nil, false
	}
	return s.arena[e.origFrom:e.origTo], true
}

func (s *Session) Kind(suffixUpper []byte) (entity.Type, bool) {
	if len(suffixUpper) > suffixCap {
		return entity.Unknown, false
	}
	var key Suffix
	copy(key[:], suffixUpper)
	idx, ok := s.set[key]
	if !ok {
		return entity.Unknown, false
	}
	return s.entries[idx].typ, true
}

func (s *Session) Numeric(suffixUpper []byte) bool {
	if len(suffixUpper) > suffixCap {
		return false
	}
	var key Suffix
	copy(key[:], suffixUpper)
	idx, ok := s.set[key]
	return ok && s.entries[idx].numeric
}

func (s *Session) MaskJSONStrings(dst, doc []byte) ([]byte, error) {
	return s.eng.rw.Rewrite(dst, doc, s.maskStringLeaf)
}

func (s *Session) maskStringLeaf(dst []byte, _ []jsonx.Frame, raw []byte) (out []byte, changed bool, err error) {
	return s.MaskLeaf(dst, raw)
}

func (s *Session) MaskLeaf(dst, raw []byte) (out []byte, changed bool, err error) {
	if jsonx.IsString(raw) {
		return s.MaskQuoted(dst, raw)
	}
	mark := len(s.scratch)
	s.scratch, err = s.MaskText(s.scratch, raw)
	if err != nil {
		return dst, false, err
	}
	masked := s.scratch[mark:]
	if bytes.Equal(masked, raw) {
		s.scratch = s.scratch[:mark]
		return dst, false, nil
	}
	if len(s.spans) == 1 && s.spans[0].Start == 0 && s.spans[0].End == len(raw) && len(s.entries) > 0 {
		s.entries[s.last].numeric = true
	}
	dst = jsonx.Quote(dst, masked)
	s.scratch = s.scratch[:mark]
	return dst, true, nil
}

func (s *Session) MaskQuoted(dst, raw []byte) (out []byte, changed bool, err error) {
	s.scratch, err = jsonx.Unquote(s.scratch[:0], raw)
	if err != nil {
		return dst, false, err
	}
	mark := len(s.scratch)
	s.scratch, err = s.MaskText(s.scratch, s.scratch[:mark])
	if err != nil {
		return dst, false, err
	}
	masked := s.scratch[mark:]
	if bytes.Equal(masked, s.scratch[:mark]) {
		return dst, false, nil
	}
	return jsonx.Quote(dst, masked), true, nil
}

const maxPooledArena = 1 << 20

func (s *Session) Release() {
	clear(s.arena)
	clear(s.scratch)
	clear(s.canon)
	clear(s.entries)
	clear(s.spans)
	clear(s.set)
	s.arena = s.arena[:0]
	s.entries = s.entries[:0]
	s.spans = s.spans[:0]
	s.scratch = s.scratch[:0]
	s.canon = s.canon[:0]
	s.variants = 0
	s.blocked = [entity.Count]bool{}
	s.counters = Counters{}
	s.tenant = nil
	s.phones = nil
	if cap(s.arena) > maxPooledArena || cap(s.scratch) > maxPooledArena {
		return
	}
	s.eng.pool.Put(s)
}
