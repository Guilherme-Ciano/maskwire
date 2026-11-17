package token

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"sync"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

const (
	MinMasterKeyLen = 32
	DefaultLength   = 6
	MinLength       = 5
	MaxLength       = 10
	crockford       = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var ErrShortMasterKey = errors.New("master key must be at least 32 bytes")

type Keyring struct {
	master  []byte
	version uint16
}

func NewKeyring(master []byte, version uint16) (*Keyring, error) {
	if len(master) < MinMasterKeyLen {
		return nil, ErrShortMasterKey
	}
	k := &Keyring{master: make([]byte, len(master)), version: version}
	copy(k.master, master)
	return k, nil
}

func (k *Keyring) AuditKey() ([]byte, error) {
	key, err := hkdf.Key(sha256.New, k.master, nil, "maskwire-audit-v1", 32)
	if err != nil {
		return nil, fmt.Errorf("derive audit key: %w", err)
	}
	return key, nil
}

func (k *Keyring) Version() uint16 {
	return k.version
}

func (k *Keyring) Tenant(id string) (*TenantKey, error) {
	key, err := hkdf.Key(sha256.New, k.master, []byte(id), "maskwire-token-v"+strconv.Itoa(int(k.version)), 32)
	if err != nil {
		return nil, fmt.Errorf("derive tenant key: %w", err)
	}
	return &TenantKey{key: key}, nil
}

type TenantKey struct {
	key  []byte
	pool sync.Pool
}

func (k *TenantKey) Sum(t entity.Type, canon []byte) [32]byte {
	h, ok := k.pool.Get().(hash.Hash)
	if !ok {
		h = hmac.New(sha256.New, k.key)
	}
	h.Reset()
	h.Write([]byte{byte(t), 0})
	h.Write(canon)
	var out [32]byte
	h.Sum(out[:0])
	k.pool.Put(h)
	return out
}

func (k *TenantKey) Derive(scope []byte) *TenantKey {
	m := hmac.New(sha256.New, k.key)
	m.Write([]byte("maskwire-scope-v1"))
	m.Write([]byte{0})
	m.Write(scope)
	return &TenantKey{key: m.Sum(nil)}
}

func (k *TenantKey) Secret(label string) []byte {
	m := hmac.New(sha256.New, k.key)
	m.Write([]byte("maskwire-secret-v1"))
	m.Write([]byte{0})
	m.Write([]byte(label))
	return m.Sum(nil)
}

func AppendSuffix(dst []byte, sum *[32]byte, n int) []byte {
	n = min(max(n, 1), MaxLength)
	var acc uint64
	bits := 0
	idx := 0
	for range n {
		for bits < 5 {
			acc = acc<<8 | uint64(sum[idx])
			idx++
			bits += 8
		}
		bits -= 5
		dst = append(dst, crockford[(acc>>uint(bits))&31])
	}
	return dst
}

func AppendToken(dst []byte, t entity.Type, suffix []byte) []byte {
	dst = append(dst, '[')
	dst = append(dst, t.Label()...)
	dst = append(dst, '_')
	dst = append(dst, suffix...)
	return append(dst, ']')
}

func AppendRedacted(dst []byte, t entity.Type) []byte {
	dst = append(dst, "[REDACTED_"...)
	dst = append(dst, t.Label()...)
	return append(dst, ']')
}

func CrockfordFold(c byte) (byte, bool) {
	switch {
	case c >= 'a' && c <= 'z':
		c -= 'a' - 'A'
	case c >= '0' && c <= '9':
		return c, true
	}
	switch c {
	case 'I', 'L', 'O', 'U':
		return 0, false
	}
	if c >= 'A' && c <= 'Z' {
		return c, true
	}
	return 0, false
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func foldWide(raw []byte) []byte {
	if bytes.IndexByte(raw, 0xEF) < 0 {
		return raw
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if i+2 < len(raw) && raw[i] == 0xEF && raw[i+1] == 0xBC && raw[i+2] >= 0x90 && raw[i+2] <= 0x99 {
			out = append(out, '0'+raw[i+2]-0x90)
			i += 2
			continue
		}
		out = append(out, raw[i])
	}
	return out
}

func appendPersonCanonical(dst, raw []byte) []byte {
	start := len(dst)
	space := false
	for _, c := range raw {
		if c == ' ' || c == '\t' || c == '\n' {
			space = len(dst) > start
			continue
		}
		if space {
			dst = append(dst, ' ')
			space = false
		}
		dst = append(dst, lower(c))
	}
	return dst
}

func AppendCanonical(dst []byte, t entity.Type, raw []byte) []byte {
	raw = foldWide(raw)
	switch t {
	case entity.Email:
		for _, c := range raw {
			dst = append(dst, lower(c))
		}
	case entity.IP, entity.IPPrivate:
		for _, c := range raw {
			dst = append(dst, lower(c))
		}
	case entity.Phone, entity.CPF, entity.PTNIF, entity.DESteuerID, entity.CreditCard, entity.NLBSN, entity.PLPESEL,
		entity.BENN, entity.ATSVNR, entity.SEPNR, entity.BRPIS:
		for _, c := range raw {
			if isDigit(c) {
				dst = append(dst, c)
			}
		}
	case entity.CNPJ, entity.ESDNI, entity.ESNIE, entity.FRNIR, entity.ITCF, entity.IBAN, entity.FIHETU,
		entity.IEPPSN, entity.UKNINO:
		for _, c := range raw {
			c = upper(c)
			if isDigit(c) || (c >= 'A' && c <= 'Z') {
				dst = append(dst, c)
			}
		}
	case entity.Person:
		dst = appendPersonCanonical(dst, raw)
	case entity.Unknown:
		dst = append(dst, raw...)
	}
	return dst
}
