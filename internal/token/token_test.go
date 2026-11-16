package token

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func tenant(t *testing.T, master byte, version uint16, id string) *TenantKey {
	t.Helper()
	kr, err := NewKeyring(bytes.Repeat([]byte{master}, 32), version)
	if err != nil {
		t.Fatal(err)
	}
	k, err := kr.Tenant(id)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestShortMasterKey(t *testing.T) {
	t.Parallel()
	if _, err := NewKeyring(make([]byte, 31), 1); !errors.Is(err, ErrShortMasterKey) {
		t.Fatalf("got %v", err)
	}
}

func TestSuffixIsDeterministicAndKeyed(t *testing.T) {
	t.Parallel()
	canon := []byte("52998224725")
	a := tenant(t, 1, 1, "acme")
	s1, s2 := a.Sum(entity.CPF, canon), a.Sum(entity.CPF, canon)
	if s1 != s2 {
		t.Fatal("same input must give the same sum")
	}
	for name, other := range map[string][32]byte{
		"tenant":  tenant(t, 1, 1, "globex").Sum(entity.CPF, canon),
		"version": tenant(t, 1, 2, "acme").Sum(entity.CPF, canon),
		"master":  tenant(t, 2, 1, "acme").Sum(entity.CPF, canon),
		"type":    a.Sum(entity.PTNIF, canon),
	} {
		if other == s1 {
			t.Errorf("changing the %s must change the token", name)
		}
	}
}

func TestSuffixAlphabetAndLength(t *testing.T) {
	t.Parallel()
	sum := tenant(t, 1, 1, "acme").Sum(entity.Email, []byte("ana@x.pt"))
	for n := range MaxLength + 3 {
		out := AppendSuffix(nil, &sum, n)
		want := min(max(n, 1), MaxLength)
		if len(out) != want {
			t.Fatalf("n=%d: len %d", n, len(out))
		}
		for _, c := range out {
			if !strings.ContainsRune(crockford, rune(c)) {
				t.Fatalf("non-Crockford char %q", c)
			}
		}
	}
	short := AppendSuffix(nil, &sum, 6)
	if !bytes.HasPrefix(AppendSuffix(nil, &sum, 8), short) {
		t.Fatal("longer suffixes must extend shorter ones")
	}
}

func TestTokenShapes(t *testing.T) {
	t.Parallel()
	if got := string(AppendToken(nil, entity.DESteuerID, []byte("ABC123"))); got != "[STEUERID_ABC123]" {
		t.Fatalf("got %s", got)
	}
	if got := string(AppendRedacted(nil, entity.Email)); got != "[REDACTED_EMAIL]" {
		t.Fatalf("got %s", got)
	}
	for _, c := range []byte("ILOU") {
		if _, ok := CrockfordFold(c); ok {
			t.Errorf("%c must not be Crockford", c)
		}
	}
	if u, ok := CrockfordFold('k'); !ok || u != 'K' {
		t.Fatal("lower case must fold")
	}
}

func TestCanonicalForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		t        entity.Type
		raw, out string
	}{
		{entity.Email, "Ana.Souza@Correio.PT", "ana.souza@correio.pt"},
		{entity.CPF, "529.982.247-25", "52998224725"},
		{entity.CPF, "５２９.９８２.２４７-２５", "52998224725"},
		{entity.IBAN, "pt50 0002 0123 1234 5678 9015 4", "PT50000201231234567890154"},
		{entity.CNPJ, "12.abc.345/01de-35", "12ABC34501DE35"},
		{entity.Phone, "+351 912-345-678", "351912345678"},
		{entity.IP, "2001:DB8::1", "2001:db8::1"},
	}
	for _, tc := range cases {
		if got := string(AppendCanonical(nil, tc.t, []byte(tc.raw))); got != tc.out {
			t.Errorf("%s %q: got %q want %q", tc.t, tc.raw, got, tc.out)
		}
	}
}

func TestAuditKeyDiffersFromTenantKeys(t *testing.T) {
	t.Parallel()
	kr, err := NewKeyring(bytes.Repeat([]byte{1}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	a, err := kr.AuditKey()
	if err != nil || len(a) != 32 {
		t.Fatalf("audit key: %v", err)
	}
	b, err := kr.AuditKey()
	if err != nil || !bytes.Equal(a, b) || kr.Version() != 1 {
		t.Fatal("audit key must be stable")
	}
}

func TestDerive(t *testing.T) {
	kr, err := NewKeyring(bytes.Repeat([]byte{7}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	base, err := kr.Tenant("acme")
	if err != nil {
		t.Fatal(err)
	}
	a1, a2, b := base.Derive([]byte("a")), base.Derive([]byte("a")), base.Derive([]byte("b"))
	canon := []byte("ana@x.pt")
	if a1.Sum(entity.Email, canon) != a2.Sum(entity.Email, canon) {
		t.Fatal("same scope gives different sums")
	}
	if a1.Sum(entity.Email, canon) == b.Sum(entity.Email, canon) ||
		a1.Sum(entity.Email, canon) == base.Sum(entity.Email, canon) {
		t.Fatal("scopes are not separated")
	}
}
