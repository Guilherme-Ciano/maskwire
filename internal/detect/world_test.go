package detect

import (
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func TestWorldIDsInText(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	cases := []struct {
		in   string
		want string
		typ  entity.Type
	}{
		{"SSN 123-45-6789 on file", "123-45-6789", entity.USSSN},
		{"employee 123-45-6789 on file", "123-45-6789", entity.USSSN},
		{"ITIN 912-70-1234 please", "912-70-1234", entity.USITIN},
		{"EIN 12-3456789 for payroll", "12-3456789", entity.USEIN},
		{"routing number 021000021 ok", "021000021", entity.USABA},
		{"CURP BADD110313HCMLNS06 ok", "BADD110313HCMLNS06", entity.MXCURP},
		{"ver BADD110313HCMLNS06 ok", "BADD110313HCMLNS06", entity.MXCURP},
		{"RFC GODE561231GR8 ok", "GODE561231GR8", entity.MXRFC},
		{"CUIT 20-12345678-6 ok", "20-12345678-6", entity.ARCUIT},
		{"RUT 12.345.678-5 ok", "12.345.678-5", entity.CLRUT},
		{"rut 1.000.005-K ok", "1.000.005-K", entity.CLRUT},
		{"NIT 800.197.268-4 ok", "800.197.268-4", entity.CONIT},
		{"RUC 20123456786 ok", "20123456786", entity.PERUC},
		{"cédula 1.234.567-2 ok", "1.234.567-2", entity.UYCI},
		{"Aadhaar 2341 2341 2346 ok", "2341 2341 2346", entity.INAadhaar},
		{"PAN ABCPE1234F ok", "ABCPE1234F", entity.INPAN},
		{"TFN 123 456 782 ok", "123 456 782", entity.AUTFN},
		{"ABN 51 824 753 556 ok", "51 824 753 556", entity.AUABN},
		{"IRD 49-091-850 ok", "49-091-850", entity.NZIRD},
		{"マイナンバー 1234 5678 9018 です", "1234 5678 9018", entity.JPMyNumber},
		{"주민등록번호 800101-1000008 입니다", "800101-1000008", entity.KRRRN},
		{"身份证 11010519491231002X 号", "11010519491231002X", entity.CNRIC},
		{"NRIC S1234567D ok", "S1234567D", entity.SGNRIC},
		{"ID number 8001015009087 ok", "8001015009087", entity.ZAID},
		{"teudat zehut 123456782 ok", "123456782", entity.ILID},
		{"TC Kimlik No 10000000146 ok", "10000000146", entity.TRTCKN},
	}
	for _, tc := range cases {
		var got []string
		ok := false
		for _, sp := range d.Detect(nil, []byte(tc.in)) {
			got = append(got, sp.Type.String()+":"+tc.in[sp.Start:sp.End])
			if sp.Type == tc.typ && tc.in[sp.Start:sp.End] == tc.want {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%q: want %s %q, got %v", tc.in, tc.typ, tc.want, got)
		}
	}
}

func TestWorldIDsNeedContext(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	contextOnly := map[entity.Type]bool{
		entity.USEIN: true, entity.ZAID: true, entity.USABA: true, entity.AUTFN: true, entity.MXRFC: true,
	}
	for _, in := range []string{
		"order 12-3456789 shipped",
		"barcode 8001015009087 in stock",
		"total 021000021 units",
		"invoice 123 456 782 paid",
		"item 670302457 today",
		"Hospital Albert Einstein ref 659426838",
	} {
		for _, sp := range d.Detect(nil, []byte(in)) {
			if contextOnly[sp.Type] {
				t.Errorf("%q: unexpected %s %q", in, sp.Type, in[sp.Start:sp.End])
			}
		}
	}
}
