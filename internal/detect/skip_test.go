package detect

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestSkipKeepsDetections(t *testing.T) {
	t.Parallel()
	fast := New(Options{Names: true})
	slow := New(Options{Names: true})
	slow.exhaustive = true
	pieces := []string{
		"hello", "the", "team", "Hello", "Tuesday", "OK", "Q3", "We", "AKIAIOSFODNN7EXAMPLE", "ghp_",
		"Ana", "Souza", "cpf", "CPF", "529.982.247-25", "52998224725", "tel", "+55 11 98765-4321", "(11) 98765-4321",
		"ana@x.com.br", "PT50 0002 0123 1234 5678 9015 4", "4242 4242 4242 4242", "8.8.8.8", "::1", "fe80::1",
		"password=", "Xk9#mP2$vL8@qR5!", "eyJhbGciOiJIUzI1NiJ9", ".", "eyJzdWIiOiIxIn0", "sk_" + "live_abcdefghijklmnop",
		" ", " ", " ", ", ", ". ", ": ", "=", "-", "_", "\n", "é", "Ünal", "camelCase", "dob", "15/03/1985",
		"12345678", "x", "Sr.", "Dr", " ", "１２３", "ein", "12-3456789", "NIF", "123456789", "DE89370400440532013000",
	}
	rng := rand.New(rand.NewPCG(9, 13))
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.IntN(24) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
			if rng.IntN(3) > 0 {
				b.WriteByte(' ')
			}
		}
		text := []byte(b.String())
		got := fast.Detect(nil, text)
		want := slow.Detect(nil, text)
		if !slices.Equal(got, want) {
			t.Fatalf("%q\nskip:       %+v\nexhaustive: %+v", text, got, want)
		}
	}
}
