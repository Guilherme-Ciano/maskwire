package detect

import (
	"hash/maphash"
	"sync"
)

const phoneCacheSize = 4096

type phoneCache struct {
	mu    sync.RWMutex
	seed  maphash.Seed
	known map[uint64]bool
}

func newPhoneCache() *phoneCache {
	return &phoneCache{seed: maphash.MakeSeed(), known: make(map[uint64]bool, phoneCacheSize)}
}

func (c *phoneCache) key(raw []byte, plus bool) uint64 {
	k := maphash.Bytes(c.seed, raw)
	if plus {
		k ^= 1
	}
	return k
}

func (c *phoneCache) get(k uint64) (valid, ok bool) {
	c.mu.RLock()
	valid, ok = c.known[k]
	c.mu.RUnlock()
	return valid, ok
}

func (c *phoneCache) put(k uint64, valid bool) {
	c.mu.Lock()
	if len(c.known) >= phoneCacheSize {
		clear(c.known)
	}
	c.known[k] = valid
	c.mu.Unlock()
}
