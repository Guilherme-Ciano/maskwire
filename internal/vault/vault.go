package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const (
	keyPrefix     = "mw:v1:"
	lookupTimeout = 100 * time.Millisecond
	maxValueLen   = 4096
)

var ErrCorrupt = errors.New("vault entry failed authentication")

type Store interface {
	MGet(ctx context.Context, keys []string) ([][]byte, error)
	SetMany(ctx context.Context, items map[string][]byte, ttl time.Duration) error
}

type Vault struct {
	store Store
	ttl   time.Duration
}

func New(store Store, ttl time.Duration) *Vault {
	return &Vault{store: store, ttl: ttl}
}

type item struct {
	typ     entity.Type
	numeric bool
	orig    []byte
	ok      bool
}

type Scope struct {
	v     *Vault
	aead  cipher.AEAD
	ns    string
	cache map[string]item
}

func (v *Vault) Open(k *token.TenantKey) (*Scope, error) {
	block, err := aes.NewCipher(k.Secret("vault-aead"))
	if err != nil {
		return nil, fmt.Errorf("vault cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault gcm: %w", err)
	}
	return &Scope{
		v:     v,
		aead:  aead,
		ns:    keyPrefix + hex.EncodeToString(k.Secret("vault-ns")[:16]) + ":",
		cache: map[string]item{},
	}, nil
}

func (sc *Scope) key(suffix string) string {
	return sc.ns + suffix
}

func (sc *Scope) seal(suffix string, t entity.Type, orig []byte, numeric bool) []byte {
	plain := make([]byte, 0, 2+len(orig))
	flag := byte(0)
	if numeric {
		flag = 1
	}
	plain = append(append(plain, byte(t), flag), orig...)
	nonce := make([]byte, sc.aead.NonceSize(), sc.aead.NonceSize()+len(plain)+sc.aead.Overhead())
	_, _ = rand.Read(nonce)
	return sc.aead.Seal(nonce, nonce, plain, []byte(sc.key(suffix)))
}

func (sc *Scope) open(suffix string, sealed []byte) (item, error) {
	ns := sc.aead.NonceSize()
	if len(sealed) < ns+sc.aead.Overhead()+2 {
		return item{}, ErrCorrupt
	}
	plain, err := sc.aead.Open(nil, sealed[:ns], sealed[ns:], []byte(sc.key(suffix)))
	if err != nil || len(plain) < 2 || int(plain[0]) >= entity.Count {
		return item{}, ErrCorrupt
	}
	return item{typ: entity.Type(plain[0]), numeric: plain[1] == 1, orig: plain[2:], ok: true}, nil
}

func (sc *Scope) Save(ctx context.Context, s *mask.Session) error {
	items := map[string][]byte{}
	s.Each(func(suffix []byte, t entity.Type, orig []byte, numeric bool) {
		if len(orig) <= maxValueLen {
			items[sc.key(string(suffix))] = sc.seal(string(suffix), t, orig, numeric)
		}
	})
	if len(items) == 0 {
		return nil
	}
	if err := sc.v.store.SetMany(ctx, items, sc.v.ttl); err != nil {
		return fmt.Errorf("vault save: %w", err)
	}
	return nil
}

func (sc *Scope) get(suffix []byte) item {
	if it, ok := sc.cache[string(suffix)]; ok {
		return it
	}
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	var it item
	vals, err := sc.v.store.MGet(ctx, []string{sc.key(string(suffix))})
	if err == nil && len(vals) == 1 && vals[0] != nil {
		if opened, oerr := sc.open(string(suffix), vals[0]); oerr == nil {
			it = opened
		}
	}
	sc.cache[string(suffix)] = it
	return it
}

type Set struct {
	Session *mask.Session
	Scope   *Scope
}

func (v Set) Lookup(t entity.Type, suffixUpper []byte) ([]byte, bool) {
	if orig, ok := v.Session.Lookup(t, suffixUpper); ok {
		return orig, true
	}
	it := v.Scope.get(suffixUpper)
	if !it.ok || (t != entity.Unknown && it.typ.Label() != t.Label()) {
		return nil, false
	}
	return it.orig, true
}

func (v Set) Numeric(suffixUpper []byte) bool {
	if v.Session.Numeric(suffixUpper) {
		return true
	}
	it := v.Scope.get(suffixUpper)
	return it.ok && it.numeric
}

func (v Set) Kind(suffixUpper []byte) (entity.Type, bool) {
	if t, ok := v.Session.Kind(suffixUpper); ok {
		return t, true
	}
	it := v.Scope.get(suffixUpper)
	return it.typ, it.ok
}
