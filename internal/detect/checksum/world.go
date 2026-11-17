package checksum

func digit(c byte) int {
	return int(c - '0')
}

func weighted(b []byte, weights ...int) int {
	sum := 0
	for i, w := range weights {
		sum += digit(b[i]) * w
	}
	return sum
}

func number(b []byte) int {
	n := 0
	for _, c := range b {
		n = n*10 + digit(c)
	}
	return n
}

func USSSN(b []byte) bool {
	if len(b) != 9 || !allDigits(b) {
		return false
	}
	area, group, serial := number(b[:3]), number(b[3:5]), number(b[5:])
	return area != 0 && area != 666 && area < 900 && group != 0 && serial != 0
}

func USITIN(b []byte) bool {
	if len(b) != 9 || !allDigits(b) || b[0] != '9' {
		return false
	}
	g := number(b[3:5])
	return (g >= 50 && g <= 65) || (g >= 70 && g <= 88) || (g >= 90 && g <= 92) || (g >= 94 && g <= 99)
}

func USEIN(b []byte) bool {
	if len(b) != 9 || !allDigits(b) || allSame(b) {
		return false
	}
	switch number(b[:2]) {
	case 0, 7, 8, 9, 17, 18, 19, 28, 29, 49, 69, 70, 78, 79, 89, 96, 97:
		return false
	}
	return true
}

func USABA(b []byte) bool {
	if len(b) != 9 || !allDigits(b) || allSame(b) {
		return false
	}
	return weighted(b, 3, 7, 1, 3, 7, 1, 3, 7, 1)%10 == 0
}

func curpValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'N':
		return int(c-'A') + 10
	case c >= 'O' && c <= 'Z':
		return int(c-'O') + 25
	}
	return -1
}

func letters(b []byte) bool {
	for _, c := range b {
		if !isUpper(c) {
			return false
		}
	}
	return len(b) > 0
}

func validYYMMDD(b []byte) bool {
	return validDate(two(b[4:6]), two(b[2:4]))
}

func MXCURP(b []byte) bool {
	if len(b) != 18 || !letters(b[:4]) || !allDigits(b[4:10]) || !validYYMMDD(b[4:10]) {
		return false
	}
	if b[10] != 'H' && b[10] != 'M' && b[10] != 'X' {
		return false
	}
	if !letters(b[11:16]) || !isDigit(b[17]) {
		return false
	}
	sum := 0
	for i, c := range b[:17] {
		v := curpValue(c)
		if v < 0 {
			return false
		}
		sum += v * (18 - i)
	}
	return (10-sum%10)%10 == digit(b[17])
}

func rfcValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'N':
		return int(c-'A') + 10
	case c == '&':
		return 24
	case c >= 'O' && c <= 'Z':
		return int(c-'O') + 25
	case c == ' ':
		return 37
	}
	return -1
}

func MXRFC(b []byte) bool {
	n := len(b)
	if n != 12 && n != 13 {
		return false
	}
	head := n - 9
	if !letters(b[:head]) || !allDigits(b[head:head+6]) || !validYYMMDD(b[head:head+6]) {
		return false
	}
	var buf [12]byte
	padded := buf[:0]
	if n == 12 {
		padded = append(padded, ' ')
	}
	padded = append(padded, b[:n-1]...)
	sum := 0
	for i, c := range padded {
		v := rfcValue(c)
		if v < 0 {
			return false
		}
		sum += v * (13 - i)
	}
	r := sum % 11
	want := byte('0')
	switch {
	case r == 0:
	case 11-r == 10:
		want = 'A'
	default:
		want = byte('0' + 11 - r)
	}
	return b[n-1] == want
}

func ARCUIT(b []byte) bool {
	if len(b) != 11 || !allDigits(b) {
		return false
	}
	switch number(b[:2]) {
	case 20, 23, 24, 27, 30, 33, 34:
	default:
		return false
	}
	c := 11 - weighted(b, 5, 4, 3, 2, 7, 6, 5, 4, 3, 2)%11
	switch c {
	case 11:
		c = 0
	case 10:
		return false
	}
	return c == digit(b[10])
}

func CLRUT(b []byte) bool {
	n := len(b)
	if n < 8 || n > 9 || !allDigits(b[:n-1]) || b[0] == '0' {
		return false
	}
	sum, w := 0, 2
	for i := n - 2; i >= 0; i-- {
		sum += digit(b[i]) * w
		w++
		if w > 7 {
			w = 2
		}
	}
	var want byte
	switch r := 11 - sum%11; r {
	case 11:
		want = '0'
	case 10:
		want = 'K'
	default:
		want = byte('0' + r)
	}
	return b[n-1] == want
}

func CONIT(b []byte) bool {
	if len(b) != 10 || !allDigits(b) || allSame(b) {
		return false
	}
	weights := [9]int{41, 37, 29, 23, 19, 17, 13, 7, 3}
	sum := 0
	for i, w := range weights {
		sum += digit(b[i]) * w
	}
	r := sum % 11
	if r >= 2 {
		r = 11 - r
	}
	return r == digit(b[9])
}

func PERUC(b []byte) bool {
	if len(b) != 11 || !allDigits(b) {
		return false
	}
	switch number(b[:2]) {
	case 10, 15, 16, 17, 20:
	default:
		return false
	}
	c := 11 - weighted(b, 5, 4, 3, 2, 7, 6, 5, 4, 3, 2)%11
	switch c {
	case 10:
		c = 0
	case 11:
		c = 1
	}
	return c == digit(b[10])
}

func UYCI(b []byte) bool {
	if len(b) != 8 || !allDigits(b) || allSame(b) {
		return false
	}
	return (10-weighted(b, 2, 9, 8, 7, 6, 3, 4)%10)%10 == digit(b[7])
}

func verhoeff(b []byte) bool {
	d := [10][10]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
		{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
		{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
		{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	p := [8][10]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
		{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
		{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
		{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
	var c byte
	for i := range b {
		c = d[c][p[i%8][b[len(b)-1-i]-'0']]
	}
	return c == 0
}

func INAadhaar(b []byte) bool {
	return len(b) == 12 && allDigits(b) && b[0] >= '2' && !allSame(b[1:]) && verhoeff(b)
}

func INPAN(b []byte) bool {
	if len(b) != 10 || !letters(b[:5]) || !allDigits(b[5:9]) || !isUpper(b[9]) {
		return false
	}
	switch b[3] {
	case 'P', 'C', 'H', 'F', 'A', 'T', 'B', 'L', 'J', 'G':
		return true
	}
	return false
}

func AUTFN(b []byte) bool {
	switch {
	case len(b) == 9 && allDigits(b) && !allSame(b):
		return weighted(b, 1, 4, 3, 7, 5, 8, 6, 9, 10)%11 == 0
	case len(b) == 8 && allDigits(b) && !allSame(b):
		return weighted(b, 10, 7, 8, 4, 6, 3, 5, 1)%11 == 0
	}
	return false
}

func AUABN(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || b[0] == '0' {
		return false
	}
	weights := [11]int{10, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
	sum := (digit(b[0]) - 1) * weights[0]
	for i := 1; i < 11; i++ {
		sum += digit(b[i]) * weights[i]
	}
	return sum%89 == 0
}

func NZIRD(b []byte) bool {
	if (len(b) != 8 && len(b) != 9) || !allDigits(b) {
		return false
	}
	var buf [9]byte
	copy(buf[9-len(b):], b)
	for i := range 9 - len(b) {
		buf[i] = '0'
	}
	n := number(buf[:8])
	if n < 1_000_000 || n > 15_000_000 {
		return false
	}
	for _, w := range [][8]int{{3, 2, 7, 6, 5, 4, 3, 2}, {7, 4, 3, 2, 5, 2, 7, 6}} {
		r := weighted(buf[:8], w[:]...) % 11
		c := 0
		if r != 0 {
			c = 11 - r
		}
		if c != 10 {
			return c == digit(buf[8])
		}
	}
	return false
}

func JPMyNumber(b []byte) bool {
	if len(b) != 12 || !allDigits(b) || allSame(b) {
		return false
	}
	sum := 0
	for n := 1; n <= 11; n++ {
		q := n + 1
		if n > 6 {
			q = n - 5
		}
		sum += digit(b[11-n]) * q
	}
	r := sum % 11
	c := 0
	if r > 1 {
		c = 11 - r
	}
	return c == digit(b[11])
}

func KRRRN(b []byte) bool {
	if len(b) != 13 || !allDigits(b) || !validDate(two(b[4:6]), two(b[2:4])) || b[6] < '1' || b[6] > '8' {
		return false
	}
	c := (11 - weighted(b, 2, 3, 4, 5, 6, 7, 8, 9, 2, 3, 4, 5)%11) % 10
	return c == digit(b[12])
}

func CNRIC(b []byte) bool {
	if len(b) != 18 || !allDigits(b[:17]) || (!isDigit(b[17]) && b[17] != 'X') {
		return false
	}
	if !validDate(two(b[12:14]), two(b[10:12])) || number(b[6:8]) < 18 || number(b[6:8]) > 20 {
		return false
	}
	r := weighted(b, 7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2) % 11
	return "10X98765432"[r] == b[17]
}

func SGNRIC(b []byte) bool {
	if len(b) != 9 || !allDigits(b[1:8]) || !isUpper(b[8]) {
		return false
	}
	sum := weighted(b[1:], 2, 7, 6, 5, 4, 3, 2)
	var table string
	switch b[0] {
	case 'S':
		table = "JZIHGFEDCBA"
	case 'T':
		sum += 4
		table = "JZIHGFEDCBA"
	case 'F':
		table = "XWUTRQPNMLK"
	case 'G':
		sum += 4
		table = "XWUTRQPNMLK"
	default:
		return false
	}
	return table[sum%11] == b[8]
}

func ZAID(b []byte) bool {
	if len(b) != 13 || !allDigits(b) || !validDate(two(b[4:6]), two(b[2:4])) || b[10] > '2' {
		return false
	}
	return Luhn(b)
}

func ILID(b []byte) bool {
	return len(b) == 9 && allDigits(b) && !allSame(b) && Luhn(b)
}

func TRTCKN(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || b[0] == '0' || allSame(b) {
		return false
	}
	odd := digit(b[0]) + digit(b[2]) + digit(b[4]) + digit(b[6]) + digit(b[8])
	even := digit(b[1]) + digit(b[3]) + digit(b[5]) + digit(b[7])
	if ((odd*7-even)%10+10)%10 != digit(b[9]) {
		return false
	}
	return (odd+even+digit(b[9]))%10 == digit(b[10])
}
