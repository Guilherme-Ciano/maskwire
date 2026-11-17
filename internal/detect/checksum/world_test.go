package checksum

import "testing"

func TestWorldIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fn    func([]byte) bool
		valid []string
		bad   []string
	}{
		{
			"US_SSN",
			USSSN,
			[]string{"123456789", "078051120"},
			[]string{"000123456", "666123456", "912345678", "123004567", "123450000"},
		},
		{"US_ITIN", USITIN, []string{"912701234", "999881234"}, []string{"912691234", "123701234"}},
		{"US_EIN", USEIN, []string{"123456789", "943456789"}, []string{"073456789", "963456789"}},
		{"US_ABA", USABA, []string{"011000015", "021000021", "111000025"}, []string{"011000016", "123456789"}},
		{
			"MX_CURP",
			MXCURP,
			[]string{"BADD110313HCMLNS06", "HEGG560427MVZRRL04"},
			[]string{"BADD110313HCMLNS09", "BADD111313HCMLNS06"},
		},
		{"MX_RFC", MXRFC, []string{"GODE561231GR8"}, []string{"GODE561231GR9", "GODE561331GR8"}},
		{"CO_NIT", CONIT, []string{"8001972684"}, []string{"8001972685"}},
		{"JP_MYNUMBER", JPMyNumber, []string{"123456789018"}, []string{"123456789019"}},
		{"KR_RRN", KRRRN, []string{"8001011000008"}, []string{"8001011000006", "8013011000008"}},
		{"AR_CUIT", ARCUIT, []string{"20123456786", "33693450239"}, []string{"20123456785", "11123456786"}},
		{"CL_RUT", CLRUT, []string{"123456785", "760864285", "1000005K"}, []string{"123456784", "012345678"}},
		{"UY_CI", UYCI, []string{"12345672"}, []string{"12345673"}},
		{"IN_AADHAAR", INAadhaar, []string{"234123412346"}, []string{"234123412347", "123412341234"}},
		{"IN_PAN", INPAN, []string{"ABCPE1234F"}, []string{"ABCDE1234F", "ABCP1E234F"}},
		{"AU_TFN", AUTFN, []string{"123456782", "876543210"}, []string{"123456789"}},
		{"AU_ABN", AUABN, []string{"51824753556", "53004085616"}, []string{"51824753557"}},
		{"NZ_IRD", NZIRD, []string{"49091850", "35901981", "49098576", "136410132"}, []string{"49091851"}},
		{"CN_RIC", CNRIC, []string{"11010519491231002X"}, []string{"110105194912310021", "11010519491331002X"}},
		{"SG_NRIC", SGNRIC, []string{"S1234567D", "T1234567J"}, []string{"S1234567A", "X1234567D"}},
		{"ZA_ID", ZAID, []string{"8001015009087"}, []string{"8001015009088", "8013015009087"}},
		{"IL_ID", ILID, []string{"000000018", "123456782"}, []string{"123456789"}},
		{"TR_TCKN", TRTCKN, []string{"10000000146"}, []string{"10000000147", "00000000146"}},
	}
	for _, tc := range cases {
		for _, v := range tc.valid {
			if !tc.fn([]byte(v)) {
				t.Errorf("%s: %s should be valid", tc.name, v)
			}
		}
		for _, v := range tc.bad {
			if tc.fn([]byte(v)) {
				t.Errorf("%s: %s should be invalid", tc.name, v)
			}
		}
	}
}
