package checksum

import "slices"

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if !isDigit(c) {
			return false
		}
	}
	return true
}

func allSame(b []byte) bool {
	for i := 1; i < len(b); i++ {
		if b[i] != b[0] {
			return false
		}
	}
	return true
}

func mod11Check(sum int) byte {
	r := sum % 11
	if r < 2 {
		return 0
	}
	return byte(11 - r)
}

func CPF(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || allSame(b) {
		return false
	}
	sum := 0
	for i := range 9 {
		sum += int(b[i]-'0') * (10 - i)
	}
	if mod11Check(sum) != b[9]-'0' {
		return false
	}
	sum = 0
	for i := range 10 {
		sum += int(b[i]-'0') * (11 - i)
	}
	return mod11Check(sum) == b[10]-'0'
}

func cnpjDigit(b []byte, weightStart int) byte {
	sum := 0
	w := weightStart
	for _, c := range b {
		sum += int(c-'0') * w
		w--
		if w < 2 {
			w = 9
		}
	}
	return mod11Check(sum)
}

func CNPJ(b []byte) bool {
	if len(b) != 14 {
		return false
	}
	for _, c := range b[:12] {
		if !isDigit(c) && !isUpper(c) {
			return false
		}
	}
	if !isDigit(b[12]) || !isDigit(b[13]) || allSame(b) {
		return false
	}
	if cnpjDigit(b[:12], 5) != b[12]-'0' {
		return false
	}
	return cnpjDigit(b[:13], 6) == b[13]-'0'
}

func ptNIFPrefix(b []byte) bool {
	switch b[0] {
	case '1', '2', '3', '5', '6', '8', '9':
		return true
	case '4':
		return b[1] == '5'
	case '7':
		return (b[1] >= '0' && b[1] <= '5') || b[1] == '7' || b[1] == '9'
	}
	return false
}

func PTNIF(b []byte) bool {
	if len(b) != 9 || !allDigits(b) || !ptNIFPrefix(b) {
		return false
	}
	sum := 0
	for i := range 8 {
		sum += int(b[i]-'0') * (9 - i)
	}
	return mod11Check(sum) == b[8]-'0'
}

const dniLetters = "TRWAGMYFPDXBNJZSQVHLCKE"

func dniLetter(n int) byte {
	return dniLetters[n%23]
}

func ESDNI(b []byte) bool {
	if len(b) != 9 || !allDigits(b[:8]) {
		return false
	}
	n := 0
	for _, c := range b[:8] {
		n = n*10 + int(c-'0')
	}
	return dniLetter(n) == b[8]
}

func ESNIE(b []byte) bool {
	if len(b) != 9 || !allDigits(b[1:8]) {
		return false
	}
	var n int
	switch b[0] {
	case 'X':
		n = 0
	case 'Y':
		n = 1
	case 'Z':
		n = 2
	default:
		return false
	}
	for _, c := range b[1:8] {
		n = n*10 + int(c-'0')
	}
	return dniLetter(n) == b[8]
}

func steuerDigitRule(b []byte) bool {
	var counts [10]int
	for _, c := range b[:10] {
		counts[c-'0']++
	}
	repeated := -1
	for d, n := range counts {
		switch {
		case n <= 1:
		case n <= 3 && repeated < 0:
			repeated = d
		default:
			return false
		}
	}
	if repeated < 0 {
		return false
	}
	r := byte('0' + repeated)
	for i := 0; i+2 < 10; i++ {
		if b[i] == r && b[i+1] == r && b[i+2] == r {
			return false
		}
	}
	return true
}

func DESteuerID(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || b[0] == '0' || !steuerDigitRule(b) {
		return false
	}
	product := 10
	for _, c := range b[:10] {
		sum := (int(c-'0') + product) % 10
		if sum == 0 {
			sum = 10
		}
		product = (sum * 2) % 11
	}
	check := 11 - product
	if check == 10 {
		check = 0
	}
	return byte(check) == b[10]-'0'
}

func nirMonth(b []byte) bool {
	m := int(b[0]-'0')*10 + int(b[1]-'0')
	return (m >= 1 && m <= 12) || (m >= 20 && m <= 42) || m >= 50
}

func FRNIR(b []byte) bool {
	if len(b) != 15 {
		return false
	}
	switch b[0] {
	case '1', '2', '7', '8':
	default:
		return false
	}
	if !allDigits(b[1:5]) || !allDigits(b[7:]) || !nirMonth(b[3:5]) {
		return false
	}
	var n uint64
	for i := range 13 {
		c := b[i]
		switch {
		case i == 6 && b[5] == '2' && c == 'A':
			n = n/10*10 + 1
			n = n*10 + 9
			continue
		case i == 6 && b[5] == '2' && c == 'B':
			n = n/10*10 + 1
			n = n*10 + 8
			continue
		case !isDigit(c):
			return false
		}
		n = n*10 + uint64(c-'0')
	}
	key := uint64(b[13]-'0')*10 + uint64(b[14]-'0')
	return 97-n%97 == key
}

func cfValue(c byte, odd bool) (int, bool) {
	oddTable := [36]int{
		1, 0, 5, 7, 9, 13, 15, 17, 19, 21,
		1, 0, 5, 7, 9, 13, 15, 17, 19, 21, 2, 4, 18, 20, 11, 3, 6, 8, 12, 14, 16, 10, 22, 25, 24, 23,
	}
	var idx, even int
	switch {
	case isDigit(c):
		idx = int(c - '0')
		even = idx
	case isUpper(c):
		idx = 10 + int(c-'A')
		even = int(c - 'A')
	default:
		return 0, false
	}
	if odd {
		return oddTable[idx], true
	}
	return even, true
}

func cfShape(b []byte) bool {
	for i, c := range b[:15] {
		switch i {
		case 0, 1, 2, 3, 4, 5, 8, 11:
			if !isUpper(c) {
				return false
			}
		default:
			if !isDigit(c) && !omocodia(c) {
				return false
			}
		}
	}
	switch b[8] {
	case 'A', 'B', 'C', 'D', 'E', 'H', 'L', 'M', 'P', 'R', 'S', 'T':
	default:
		return false
	}
	return isUpper(b[15])
}

func omocodia(c byte) bool {
	switch c {
	case 'L', 'M', 'N', 'P', 'Q', 'R', 'S', 'T', 'U', 'V':
		return true
	}
	return false
}

func ITCF(b []byte) bool {
	if len(b) != 16 || !cfShape(b) {
		return false
	}
	sum := 0
	for i, c := range b[:15] {
		v, ok := cfValue(c, i%2 == 0)
		if !ok {
			return false
		}
		sum += v
	}
	return byte('A'+sum%26) == b[15]
}

func IBANLength(country []byte) int {
	if len(country) != 2 {
		return 0
	}
	switch string(country) {
	case "NO":
		return 15
	case "BE":
		return 16
	case "DK", "FI", "FO", "GL", "NL":
		return 18
	case "MK", "SI":
		return 19
	case "AT", "BA", "EE", "KZ", "LT", "LU", "XK":
		return 20
	case "CH", "CR", "HR", "LI", "LV":
		return 21
	case "BG", "BH", "DE", "GB", "GE", "IE", "ME", "RS":
		return 22
	case "AE", "GI", "IL", "TL":
		return 23
	case "AD", "CZ", "ES", "MD", "PK", "RO", "SA", "SE", "SK", "TN", "VG":
		return 24
	case "PT":
		return 25
	case "IS", "TR":
		return 26
	case "FR", "GR", "IT", "MC", "MR", "SM":
		return 27
	case "AL", "AZ", "CY", "DO", "GT", "HU", "LB", "PL":
		return 28
	case "BR", "EG", "PS", "QA", "UA":
		return 29
	case "JO", "KW", "MU":
		return 30
	case "MT", "SC":
		return 31
	case "LC":
		return 32
	}
	return 0
}

func ibanFeed(r uint64, c byte) (uint64, bool) {
	switch {
	case isDigit(c):
		return (r*10 + uint64(c-'0')) % 97, true
	case isUpper(c):
		return (r*100 + uint64(c-'A'+10)) % 97, true
	}
	return r, false
}

func IBAN(b []byte) bool {
	if len(b) < 15 || len(b) > 34 || !isUpper(b[0]) || !isUpper(b[1]) || !isDigit(b[2]) || !isDigit(b[3]) {
		return false
	}
	if IBANLength(b[:2]) != len(b) {
		return false
	}
	var r uint64
	var ok bool
	for _, c := range b[4:] {
		if r, ok = ibanFeed(r, c); !ok {
			return false
		}
	}
	for _, c := range b[:4] {
		r, _ = ibanFeed(r, c)
	}
	return r == 1
}

func Luhn(b []byte) bool {
	if len(b) < 2 || !allDigits(b) {
		return false
	}
	sum := 0
	double := false
	for _, v := range slices.Backward(b) {
		d := int(v - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func prefixNum(b []byte, n int) int {
	v := 0
	for _, c := range b[:n] {
		v = v*10 + int(c-'0')
	}
	return v
}

func cardIIN(b []byte) bool {
	n := len(b)
	switch b[0] {
	case '4':
		return n == 13 || n == 16 || n == 19
	case '3':
		p2 := prefixNum(b, 2)
		if p2 == 34 || p2 == 37 {
			return n == 15
		}
		return (p2 == 36 || p2 == 38 || p2 == 39 || p2 == 30) && n >= 14 && n <= 19 || (p2 == 35 && n >= 16)
	case '5':
		p2 := prefixNum(b, 2)
		return p2 >= 51 && p2 <= 55 && n == 16
	case '2':
		p4 := prefixNum(b, 4)
		return p4 >= 2221 && p4 <= 2720 && n == 16
	case '6':
		return n >= 16 && n <= 19
	}
	return false
}

func CreditCard(b []byte) bool {
	if len(b) < 13 || len(b) > 19 || !allDigits(b) || allSame(b) {
		return false
	}
	return cardIIN(b) && Luhn(b)
}
