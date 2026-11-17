package detect

import (
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func TestNormalizeField(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"taxFileNumber": "tax file number ",
		"customer_ssn":  "customer ssn ",
		"SSN":           "ssn ",
		"tax-id":        "tax id ",
		"cpfCliente2":   "cpf cliente2 ",
		"":              "",
	} {
		var buf [maxHint]byte
		if got := string(buf[:normalizeField(&buf, []byte(in))]); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestFieldHints(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	cases := []struct {
		field, value string
		want         entity.Type
	}{
		{"ein", "123456789", entity.USEIN},
		{"employerIdentificationNumber", "12-3456789", entity.USEIN},
		{"taxFileNumber", "123456782", entity.AUTFN},
		{"routing_number", "021000021", entity.USABA},
		{"customer_ssn", "123456789", entity.USSSN},
		{"cpf", "52998224725", entity.CPF},
		{"order_id", "52998224725", entity.Unknown},
		{"order_id", "529.982.247-25", entity.CPF},
		{"note", "52998224725", entity.CPF},
	}
	for _, tc := range cases {
		st := PhoneState{Budget: 8}
		spans := d.DetectField(nil, []byte(tc.value), []byte(tc.field), &st)
		got := entity.Unknown
		if len(spans) == 1 {
			got = spans[0].Type
		}
		if got != tc.want {
			t.Errorf("%s=%q: got %v want %v (%v)", tc.field, tc.value, got, tc.want, spans)
		}
	}
}
