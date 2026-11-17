package vault

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func sessionKey(t *testing.T, id string) *token.TenantKey {
	t.Helper()
	kr, err := token.NewKeyring(bytes.Repeat([]byte{9}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	k, err := kr.Tenant("acme")
	if err != nil {
		t.Fatal(err)
	}
	return k.Derive([]byte("session:" + id))
}

func masked(t *testing.T, k *token.TenantKey, text string) (s *mask.Session, out string) {
	t.Helper()
	s = mask.NewEngine(detect.New(detect.Options{}), 0).NewSession(&mask.Tenant{
		ID: "acme", Key: k, Policy: policy.New(policy.Enforce, policy.Mask),
	})
	s.UseKey(k)
	b, err := s.MaskText(nil, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return s, string(b)
}

func suffixOf(tok string) []byte {
	return []byte(tok[strings.IndexByte(tok, '_')+1 : len(tok)-1])
}

func exercise(t *testing.T, store Store) {
	t.Helper()
	v := New(store, time.Hour)
	k := sessionKey(t, "conv-1")
	s1, out := masked(t, k, "mail ana@empresa.pt cpf 52998224725")
	sc, err := v.Open(k)
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Save(t.Context(), s1); err != nil {
		t.Fatal(err)
	}
	start := strings.Index(out, "[EMAIL_")
	if start < 0 {
		t.Fatalf("no e-mail token in %s", out)
	}
	tok := out[start : start+strings.IndexByte(out[start:], ']')+1]
	later, _ := masked(t, k, "nothing here")
	sc2, err := v.Open(k)
	if err != nil {
		t.Fatal(err)
	}
	set := Set{Session: later, Scope: sc2}
	if got, ok := set.Lookup(entity.Email, suffixOf(tok)); !ok || string(got) != "ana@empresa.pt" {
		t.Fatalf("lookup %q %v", got, ok)
	}
	if _, ok := set.Lookup(entity.IBAN, suffixOf(tok)); ok {
		t.Fatal("type mismatch accepted")
	}
	if typ, ok := set.Kind(suffixOf(tok)); !ok || typ != entity.Email {
		t.Fatalf("kind %v %v", typ, ok)
	}
	other, err := v.Open(sessionKey(t, "conv-2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := (Set{Session: later, Scope: other}).Lookup(entity.Email, suffixOf(tok)); ok {
		t.Fatal("another session read the vault")
	}
}

func TestVaultMemory(t *testing.T) {
	t.Parallel()
	exercise(t, NewMemory(100))
}

func TestVaultRedis(t *testing.T) {
	addr := os.Getenv("MASKWIRE_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("MASKWIRE_TEST_REDIS_ADDR not set")
	}
	r := NewRedis(addr, "")
	defer r.Close()
	if err := r.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	exercise(t, r)
}

func TestSealedValuesAreBoundToTheirKey(t *testing.T) {
	t.Parallel()
	k := sessionKey(t, "c")
	sc, err := New(NewMemory(10), time.Hour).Open(k)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sc.seal("ABCDEF", entity.CPF, []byte("52998224725"), true)
	if bytes.Contains(sealed, []byte("52998224725")) {
		t.Fatal("value stored in clear")
	}
	if it, err := sc.open("ABCDEF", sealed); err != nil || string(it.orig) != "52998224725" || !it.numeric {
		t.Fatalf("open %+v %v", it, err)
	}
	if _, err := sc.open("ZZZZZZ", sealed); !errors.Is(err, ErrCorrupt) {
		t.Fatal("value moved to another suffix opened")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := sc.open("ABCDEF", sealed); !errors.Is(err, ErrCorrupt) {
		t.Fatal("tampered value opened")
	}
	if _, err := sc.open("ABCDEF", []byte{1, 2}); !errors.Is(err, ErrCorrupt) {
		t.Fatal("short value opened")
	}
}

func TestMemoryExpiryAndLimit(t *testing.T) {
	t.Parallel()
	m := NewMemory(2)
	now := time.Unix(1000, 0)
	m.now = func() time.Time { return now }
	ctx := context.Background()
	if err := m.SetMany(ctx, map[string][]byte{"a": []byte("1"), "b": []byte("2")}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := m.SetMany(ctx, map[string][]byte{"c": []byte("3")}, time.Minute); !errors.Is(err, ErrFull) {
		t.Fatalf("over limit: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if got, err := m.MGet(ctx, []string{"a"}); err != nil || got[0] != nil {
		t.Fatal("expired entry returned")
	}
	if err := m.SetMany(ctx, map[string][]byte{"c": []byte("3")}, time.Minute); err != nil {
		t.Fatalf("expired entries not evicted: %v", err)
	}
}
