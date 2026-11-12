package detect

import (
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type hit struct {
	text string
	typ  entity.Type
}

func found(t *testing.T, d *Detector, in string) []hit {
	t.Helper()
	spans := d.Detect(nil, []byte(in))
	out := make([]hit, 0, len(spans))
	for _, s := range spans {
		out = append(out, hit{in[s.Start:s.End], s.Type})
	}
	return out
}

func TestDetect(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	cases := []struct {
		in   string
		want []hit
	}{
		{"nothing to see here, move along.", nil},
		{"Meu CPF é 529.982.247-25, obrigado", []hit{{"529.982.247-25", entity.CPF}}},
		{
			"cpf 52998224725 e cnpj 11.222.333/0001-81",
			[]hit{{"52998224725", entity.CPF}, {"11.222.333/0001-81", entity.CNPJ}},
		},
		{"novo CNPJ 12.ABC.345/01DE-35 ok", []hit{{"12.ABC.345/01DE-35", entity.CNPJ}}},
		{"NIF 123456789.", []hit{{"123456789", entity.PTNIF}}},
		{"DNI 12345678Z y NIE X1234567L", []hit{{"12345678Z", entity.ESDNI}, {"X1234567L", entity.ESNIE}}},
		{"Steuer-ID: 86095742719", []hit{{"86095742719", entity.DESteuerID}}},
		{"just 86095742719 alone", nil},
		{"NIR 1 84 12 76 451 089 46", []hit{{"1 84 12 76 451 089 46", entity.FRNIR}}},
		{"CF RSSMRA85T10A562S grazie", []hit{{"RSSMRA85T10A562S", entity.ITCF}}},
		{"IBAN PT50 0002 0123 1234 5678 9015 4 please", []hit{{"PT50 0002 0123 1234 5678 9015 4", entity.IBAN}}},
		{"pay DE89370400440532013000 now", []hit{{"DE89370400440532013000", entity.IBAN}}},
		{"card 4242 4242 4242 4242 exp", []hit{{"4242 4242 4242 4242", entity.CreditCard}}},
		{"mail ana.silva+x@empresa.com.br.", []hit{{"ana.silva+x@empresa.com.br", entity.Email}}},
		{"mail ana@example.com is reserved", nil},
		{"bad a@b and @x.com and x@y.c", nil},
		{"host 8.8.8.8 and 192.168.0.1", []hit{{"8.8.8.8", entity.IP}, {"192.168.0.1", entity.IPPrivate}}},
		{"v6 2001:4860:4860::8888 end", []hit{{"2001:4860:4860::8888", entity.IP}}},
		{"time 12:30:45 is not ip", nil},
		{"version 1.2.3.4.5 no", nil},
		{"ligue +55 11 98765-4321 hoje", []hit{{"+55 11 98765-4321", entity.Phone}}},
		{"ligue (11) 98765-4321 hoje", []hit{{"(11) 98765-4321", entity.Phone}}},
		{"tel 912 345 678", []hit{{"912 345 678", entity.Phone}}},
		{"order 12345 shipped", nil},
		{"token [EMAIL_K7Q2M] stays", nil},
		{"iban pt50 0002 0123 1234 5678 9015 4 low", []hit{{"pt50 0002 0123 1234 5678 9015 4", entity.IBAN}}},
		{"dni 12345678z y cf rssmra85t10a562s", []hit{{"12345678z", entity.ESDNI}, {"rssmra85t10a562s", entity.ITCF}}},
		{
			"cpf \uff15\uff12\uff19.\uff19\uff18\uff12.\uff12\uff14\uff17-\uff12\uff15 wide",
			[]hit{{"\uff15\uff12\uff19.\uff19\uff18\uff12.\uff12\uff14\uff17-\uff12\uff15", entity.CPF}},
		},
		{"cpf 529\u00ad982\u2013247\u201025", []hit{{"529\u00ad982\u2013247\u201025", entity.CPF}}},
		{"cpf 529\ufeff982\u3000247\u200e25", []hit{{"529\ufeff982\u3000247\u200e25", entity.CPF}}},
		{"tel 912\u2013345\u2013678", []hit{{"912\u2013345\u2013678", entity.Phone}}},
		{"mail jo\u00e3o.s\u00e1@acme.io ok", []hit{{"jo\u00e3o.s\u00e1@acme.io", entity.Email}}},
		{"plain words like abc def and x1 stay", nil},
		{"cpf\u00a0529.982.247\u200b-25", []hit{{"529.982.247\u200b-25", entity.CPF}}},
	}
	for _, tc := range cases {
		got := found(t, d, tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: got %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestPhoneBudgetIsShared(t *testing.T) {
	t.Parallel()
	d := New(Options{PhoneBudget: 2, Regions: []string{"PT"}})
	budget := d.PhoneBudget()
	for range 4 {
		d.DetectBudget(nil, []byte("ligue 912 345 678"), &budget)
	}
	if budget != 0 {
		t.Fatalf("budget %d must be consumed across calls", budget)
	}
	spans := d.DetectBudget(nil, []byte("ligue 912 345 678"), &budget)
	if len(spans) != 1 || spans[0].Score >= 0.85 {
		t.Fatalf("exhausted budget must still mask with low score: %+v", spans)
	}
}

func TestPhoneBudget(t *testing.T) {
	t.Parallel()
	d := New(Options{PhoneBudget: 1, Regions: []string{"PT"}})
	in := strings.Repeat("ligue 912 345 678 ", 4)
	spans := d.Detect(nil, []byte(in))
	if len(spans) != 4 {
		t.Fatalf("got %d spans", len(spans))
	}
	if spans[3].Score >= spans[0].Score {
		t.Fatalf("budget-exceeded phone must have lower score: %v", spans)
	}
}

func FuzzDetect(f *testing.F) {
	f.Add([]byte("CPF 529.982.247-25 mail a@b.pt ip 2001:db8::1 +351 912 345 678"))
	f.Add([]byte("PT50 0002 0123 1234 5678 9015 4 RSSMRA85T10A562S"))
	d := New(Options{PhoneBudget: 4})
	f.Fuzz(func(t *testing.T, b []byte) {
		spans := d.Detect(nil, b)
		prev := -1
		for _, s := range spans {
			if s.Start < 0 || s.End > len(b) || s.Start >= s.End {
				t.Fatalf("span out of bounds: %+v len=%d", s, len(b))
			}
			if s.Start < prev {
				t.Fatalf("overlap or unsorted: %+v", spans)
			}
			prev = s.End
		}
	})
}
