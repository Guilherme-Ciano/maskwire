package detect

import "sync"

const (
	regionCap = 512
	maxHits   = 32
)

type keyHit struct {
	start int
	end   int
	rules uint64
	phone bool
	short bool
	next  bool
}

type keyRegion struct {
	run  int
	from int
	to   int
	n    int
	nh   int
	ok   bool
	buf  [regionCap + keyPad]byte
	hits [maxHits]keyHit
}

type regionPool struct {
	pool sync.Pool
}

func (p *regionPool) get() *keyRegion {
	if r, ok := p.pool.Get().(*keyRegion); ok {
		r.run = 0
		return r
	}
	return &keyRegion{}
}

func (p *regionPool) put(r *keyRegion) {
	p.pool.Put(r)
}

func (s *scanner) keyRegion() *keyRegion {
	if s.region == nil {
		s.region = s.d.regionPool.get()
	}
	r := s.region
	if r.run == s.runID {
		return r
	}
	r.run = s.runID
	r.from = max(s.groups[0].start-keywordWindow-1, 0)
	r.to = s.groups[s.run-1].end
	r.n = r.to - r.from
	r.nh = 0
	r.ok = r.n <= regionCap
	if !r.ok {
		return r
	}
	f := s.d.fold
	for i, c := range s.text[r.from:r.to] {
		r.buf[i] = f.lower[c]
	}
	clear(r.buf[r.n : r.n+keyPad])
	ix := s.d.keys
	letter := r.from > 0 && f.letter[s.text[r.from-1]]
	for p := range r.n {
		c := r.buf[p]
		if !letter && ix.start[c] && !ix.hitsAt(r, p, f) {
			r.ok = false
			return r
		}
		letter = f.letter[c]
	}
	return r
}

func (s *scanner) regionMask(pos int) (keyMemo, bool) {
	if s.run == 0 || (pos < keywordWindow && s.hintLen > 0) {
		return keyMemo{}, false
	}
	r := s.keyRegion()
	from := max(pos-keywordWindow, 0)
	if !r.ok || from < r.from || pos > r.to {
		return keyMemo{}, false
	}
	var m keyMemo
	for _, h := range r.hits[:r.nh] {
		if h.start < from || h.end > pos {
			continue
		}
		m.phone = m.phone || h.phone
		if !h.short || h.end == pos || !h.next {
			m.rules |= h.rules
		}
	}
	f := s.d.fold
	if from > 0 && f.letter[s.text[from-1]] && s.d.keys.start[r.buf[from-r.from]] {
		rules, phone := s.d.keys.maskAt(r.buf[:], from-r.from, pos-r.from, f)
		m.rules |= rules
		m.phone = m.phone || phone
	}
	return m, true
}
