package checksum

import (
	"strings"
	"testing"
)

func TestValidators(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fn    func([]byte) bool
		valid []string
		bad   []string
	}{
		{
			"CPF",
			CPF,
			[]string{"52998224725", "11144477735"},
			[]string{"52998224724", "11111111111", "5299822472", "529982247255", "5299822472a"},
		},
		{
			"CNPJ",
			CNPJ,
			[]string{"11222333000181", "12ABC34501DE35"},
			[]string{"11222333000180", "12ABC34501DE36", "00000000000000", "12abc34501DE35", "1222333000181"},
		},
		{
			"PTNIF",
			PTNIF,
			[]string{"123456789", "501442600"},
			[]string{"123456788", "423456789", "403456780", "12345678"},
		},
		{"ESDNI", ESDNI, []string{"12345678Z", "00000000T"}, []string{"12345678A", "1234567Z", "X1234567L"}},
		{"ESNIE", ESNIE, []string{"X1234567L", "Y0000000Z"}, []string{"X1234567A", "A1234567L", "12345678Z"}},
		{
			"DESteuerID",
			DESteuerID,
			[]string{"86095742719", "65929970489", "47036892816"},
			[]string{"86095742718", "02476291358", "12345678901", "11145678903", "1234567890"},
		},
		{
			"FRNIR",
			FRNIR,
			[]string{"184127645108946", "269054958815780", "255081416802538"},
			[]string{"184127645108947", "384127645108946", "184137645108946", "18412764510894"},
		},
		{
			"ITCF",
			ITCF,
			[]string{"RSSMRA85T10A562S"},
			[]string{"RSSMRA85T10A562T", "RSSMRA85F10A562S", "rssmra85t10a562s", "RSSMRA85T10A562"},
		},
		{"IBAN", IBAN, []string{
			"GB82WEST12345698765432", "DE89370400440532013000", "PT50000201231234567890154",
			"FR1420041010050500013M02606", "ES9121000418450200051332", "IT60X0542811101000000123456",
			"BR1800360305000010009795493C1", "NL91ABNA0417164300", "BE68539007547034", "NO9386011117947",
		}, []string{"GB82WEST12345698765433", "DE8937040044053201300", "XX89370400440532013000", "DE89370400440532013000X", "de89370400440532013000"}},
		{
			"CreditCard",
			CreditCard,
			[]string{
				"4242424242424242",
				"5555555555554444",
				"378282246310005",
				"2223003122003222",
				"6011111111111117",
				"4000056655665556",
			},
			[]string{"4242424242424241", "1234567812345670", "424242424242", "0000000000000000"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, v := range tc.valid {
				if !tc.fn([]byte(v)) {
					t.Errorf("%s: expected valid", v)
				}
			}
			for _, v := range tc.bad {
				if tc.fn([]byte(v)) {
					t.Errorf("%s: expected invalid", v)
				}
			}
		})
	}
}

func TestNIRCorsica(t *testing.T) {
	t.Parallel()
	base := "185022A123456"
	num := strings.Replace(base, "2A", "19", 1)
	var n uint64
	for _, c := range num {
		n = n*10 + uint64(c-'0')
	}
	key := 97 - n%97
	valid := base + string(rune('0'+key/10)) + string(rune('0'+key%10))
	if !FRNIR([]byte(valid)) {
		t.Fatalf("%s: expected valid", valid)
	}
	if FRNIR([]byte(strings.Replace(valid, "2A", "2C", 1))) {
		t.Fatal("2C must be rejected")
	}
}

func TestLuhn(t *testing.T) {
	t.Parallel()
	if !Luhn([]byte("79927398713")) || Luhn([]byte("79927398710")) || Luhn([]byte("7")) {
		t.Fatal("luhn mismatch")
	}
}

func TestIBANLengthUnknown(t *testing.T) {
	t.Parallel()
	if IBANLength([]byte("ZZ")) != 0 || IBANLength([]byte("P")) != 0 {
		t.Fatal("unknown country must have zero length")
	}
}

func FuzzValidatorsNoPanic(f *testing.F) {
	f.Add([]byte("12ABC34501DE35"))
	f.Add([]byte("RSSMRA85T10A562S"))
	f.Add([]byte("2A0000000000000"))
	f.Fuzz(func(_ *testing.T, b []byte) {
		for _, fn := range []func([]byte) bool{CPF, CNPJ, PTNIF, ESDNI, ESNIE, DESteuerID, FRNIR, ITCF, IBAN, CreditCard, Luhn} {
			fn(b)
		}
	})
}
