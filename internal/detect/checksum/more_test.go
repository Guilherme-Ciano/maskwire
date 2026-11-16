package checksum

import (
	"strconv"
	"testing"
)

func pisWith(prefix string) string {
	w := []int{3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, c := range prefix {
		sum += int(c-'0') * w[i]
	}
	dv := 11 - sum%11
	if dv >= 10 {
		dv = 0
	}
	return prefix + strconv.Itoa(dv)
}

func TestMoreValidators(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fn    func([]byte) bool
		valid []string
		bad   []string
	}{
		{"NLBSN", NLBSN, []string{"111222333", "123456782"}, []string{"111222334", "11122233", "000000000"}},
		{
			"PLPESEL",
			PLPESEL,
			[]string{"44051401359", "02070803628"},
			[]string{"44051401358", "44151401354", "4405140135"},
		},
		{"BENN", BENN, []string{"85073003328", "17073003384"}, []string{"85073003329", "8507300332"}},
		{"ATSVNR", ATSVNR, []string{"1237010180"}, []string{"1238010180", "0237010180", "123701018"}},
		{"SEPNR", SEPNR, []string{"8112189876", "198112189876"}, []string{"8112189875", "8113189876", "81121898"}},
		{"FIHETU", FIHETU, []string{"131052308T"}, []string{"131052308U", "321052308T", "13105230T"}},
		{
			"IEPPSN",
			IEPPSN,
			[]string{"1234567T", "1234567TW", "1234567FA"},
			[]string{"1234567A", "123456T", "1234567TB"},
		},
		{
			"UKNINO",
			UKNINO,
			[]string{"AB123456C", "JG103759A"},
			[]string{"AB123456E", "GB123456A", "DA123456A", "AB12345C"},
		},
		{"BRPIS", BRPIS, []string{pisWith("1205641254"), pisWith("1700023450")}, []string{"11111111111", "120564125"}},
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

func FuzzMoreValidatorsNoPanic(f *testing.F) {
	f.Add([]byte("131052308T"))
	f.Add([]byte("1234567TW"))
	f.Fuzz(func(_ *testing.T, b []byte) {
		for _, fn := range []func([]byte) bool{NLBSN, PLPESEL, BENN, ATSVNR, SEPNR, FIHETU, IEPPSN, UKNINO, BRPIS} {
			fn(b)
		}
	})
}
