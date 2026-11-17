package detect

import (
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func TestNames(t *testing.T) {
	t.Parallel()
	d := New(Options{Names: true})
	cases := []struct {
		in   string
		want []string
	}{
		{"Escreva para Ana Souza amanhã.", []string{"Ana Souza"}},
		{"cliente: Gustavo Henrique Moraes, CPF 529.982.247-25", []string{"Gustavo Henrique Moraes"}},
		{"O Sr. Fagundes ligou.", []string{"Fagundes"}},
		{"Herr Müller und Frau Schmidt", []string{"Müller", "Schmidt"}},
		{"Maria da Silva Pereira pediu reembolso", []string{"Maria da Silva Pereira"}},
		{"Ana Souza ligou. Depois Souza mandou e-mail e Ana confirmou.", []string{"Ana Souza", "Souza", "Ana"}},
		{"Fui a São Paulo e a Santa Catarina em Janeiro.", nil},
		{"Rua Augusta, 100", nil},
		{"O pedido NAME foi enviado pela ACME.", nil},
		{"Jean-Pierre Dupont arrive", []string{"Jean-Pierre Dupont"}},
		{"nome: Fagundes Varela", []string{"Fagundes Varela"}},
	}
	for _, tc := range cases {
		var got []string
		for _, sp := range d.Detect(nil, []byte(tc.in)) {
			if sp.Type == entity.Person {
				got = append(got, tc.in[sp.Start:sp.End])
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
			}
		}
	}
	if sp := New(Options{}).Detect(nil, []byte("Escreva para Ana Souza")); len(sp) != 0 {
		t.Fatalf("names detected while disabled: %v", sp)
	}
}

func FuzzNames(f *testing.F) {
	for _, s := range []string{"Sr. Ana da Silva", "nome: Jean-Pierre O'Brien", "São Paulo", "Ãña Ç", "a-\xff B"} {
		f.Add(s)
	}
	d := New(Options{Names: true})
	f.Fuzz(func(t *testing.T, s string) {
		for _, sp := range d.Detect(nil, []byte(s)) {
			if sp.Start < 0 || sp.End > len(s) || sp.Start >= sp.End {
				t.Fatalf("bad span %+v in %q", sp, s)
			}
		}
	})
}
