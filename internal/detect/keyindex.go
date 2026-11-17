package detect

import (
	"bytes"
	"encoding/binary"
)

const (
	maxKeyedRules = 64
	keyBuckets    = 1024
	keyPad        = 8
)

type keyEntry struct {
	head  uint64
	mask  uint64
	word  []byte
	rules uint64
	phone bool
}

type keyIndex struct {
	buckets [keyBuckets][]keyEntry
	start   [256]bool
}

func bucket(a, b byte) int {
	return (int(a)<<5 ^ int(b)) & (keyBuckets - 1)
}

func head(w []byte) (value, mask uint64) {
	var buf [keyPad]byte
	n := copy(buf[:], w)
	value = binary.LittleEndian.Uint64(buf[:])
	mask = ^uint64(0)
	if n < keyPad {
		mask = 1<<(8*uint(n)) - 1
	}
	return value, mask
}

func newKeyIndex(rules *ruleIndex, phone [][]byte) *keyIndex {
	ix := &keyIndex{}
	for _, r := range rules.all {
		for _, w := range r.keys {
			ix.add(w).rules |= r.bit
		}
	}
	for _, w := range phone {
		ix.add(w).phone = true
	}
	return ix
}

func (ix *keyIndex) add(word []byte) *keyEntry {
	b := bucket(word[0], word[1])
	list := ix.buckets[b]
	for i := range list {
		if bytes.Equal(list[i].word, word) {
			return &list[i]
		}
	}
	ix.start[word[0]] = true
	h, m := head(word)
	ix.buckets[b] = append(list, keyEntry{head: h, mask: m, word: word})
	return &ix.buckets[b][len(list)]
}

func (ix *keyIndex) mark(l *lowered, f *foldTable) {
	letter := false
	for p := range l.n {
		c := l.buf[p]
		if !letter && ix.start[c] {
			r, ph := ix.maskAt(l.buf[:], p, l.n, f)
			l.rules |= r
			l.phone = l.phone || ph
		}
		letter = f.letter[c]
	}
}

func (ix *keyIndex) maskAt(buf []byte, p, limit int, f *foldTable) (rules uint64, phone bool) {
	at := binary.LittleEndian.Uint64(buf[p : p+keyPad])
	for _, e := range ix.buckets[bucket(buf[p], buf[p+1])] {
		end := p + len(e.word)
		if at&e.mask != e.head || end > limit || (len(e.word) > keyPad && !bytes.Equal(buf[p:end], e.word)) {
			continue
		}
		phone = phone || e.phone
		if len(e.word) > maxPrefixKeyword || end >= limit || !f.letter[buf[end]] {
			rules |= e.rules
		}
	}
	return rules, phone
}

func (ix *keyIndex) hitsAt(r *keyRegion, p int, f *foldTable) bool {
	buf := r.buf[:]
	at := binary.LittleEndian.Uint64(buf[p : p+keyPad])
	for i := range ix.buckets[bucket(buf[p], buf[p+1])] {
		e := &ix.buckets[bucket(buf[p], buf[p+1])][i]
		end := p + len(e.word)
		if at&e.mask != e.head || end > r.n || (len(e.word) > keyPad && !bytes.Equal(buf[p:end], e.word)) {
			continue
		}
		if r.nh == maxHits {
			return false
		}
		r.hits[r.nh] = keyHit{
			start: r.from + p,
			end:   r.from + end,
			rules: e.rules,
			phone: e.phone,
			short: len(e.word) <= maxPrefixKeyword,
			next:  end < r.n && f.letter[buf[end]],
		}
		r.nh++
	}
	return true
}
