package checksum

func NLBSN(b []byte) bool {
	if len(b) != 9 || !allDigits(b) || allSame(b) {
		return false
	}
	sum := 0
	for i := range 8 {
		sum += int(b[i]-'0') * (9 - i)
	}
	sum -= int(b[8] - '0')
	return sum%11 == 0
}

func PLPESEL(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || allSame(b) {
		return false
	}
	weights := [10]int{1, 3, 7, 9, 1, 3, 7, 9, 1, 3}
	sum := 0
	for i, w := range weights {
		sum += int(b[i]-'0') * w
	}
	if (10-sum%10)%10 != int(b[10]-'0') {
		return false
	}
	month := int(b[2]-'0')*10 + int(b[3]-'0')
	return month%20 >= 1 && month%20 <= 12
}

func BENN(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || allSame(b) {
		return false
	}
	var n uint64
	for _, c := range b[:9] {
		n = n*10 + uint64(c-'0')
	}
	key := uint64(b[9]-'0')*10 + uint64(b[10]-'0')
	if 97-n%97 == key {
		return true
	}
	return 97-(2000000000+n)%97 == key
}

func ATSVNR(b []byte) bool {
	if len(b) != 10 || !allDigits(b) || b[0] == '0' {
		return false
	}
	weights := [10]int{3, 7, 9, 0, 5, 8, 4, 2, 1, 6}
	sum := 0
	for i, w := range weights {
		sum += int(b[i]-'0') * w
	}
	check := sum % 11
	return check != 10 && check == int(b[3]-'0')
}

func validDate(dd, mm int) bool {
	return dd >= 1 && dd <= 31 && mm >= 1 && mm <= 12
}

func two(b []byte) int {
	return int(b[0]-'0')*10 + int(b[1]-'0')
}

func SEPNR(b []byte) bool {
	switch len(b) {
	case 12:
		b = b[2:]
	case 10:
	default:
		return false
	}
	if !allDigits(b) || !validDate(two(b[4:6])%60, two(b[2:4])) {
		return false
	}
	return Luhn(b)
}

const hetuChars = "0123456789ABCDEFHJKLMNPRSTUVWXY"

func FIHETU(b []byte) bool {
	if len(b) != 10 || !allDigits(b[:9]) || !validDate(two(b[0:2]), two(b[2:4])) {
		return false
	}
	n := 0
	for _, c := range b[:9] {
		n = n*10 + int(c-'0')
	}
	return hetuChars[n%31] == b[9]
}

func IEPPSN(b []byte) bool {
	if len(b) != 8 && len(b) != 9 {
		return false
	}
	if !allDigits(b[:7]) || !isUpper(b[7]) {
		return false
	}
	sum := 0
	for i := range 7 {
		sum += int(b[i]-'0') * (8 - i)
	}
	if len(b) == 9 {
		if !isUpper(b[8]) {
			return false
		}
		if b[8] != 'W' {
			sum += int(b[8]-'A'+1) * 9
		}
	}
	r := sum % 23
	want := byte('W')
	if r > 0 {
		want = byte('A' + r - 1)
	}
	return b[7] == want
}

func UKNINO(b []byte) bool {
	if len(b) != 9 || !isUpper(b[0]) || !isUpper(b[1]) || !allDigits(b[2:8]) {
		return false
	}
	switch b[0] {
	case 'D', 'F', 'I', 'Q', 'U', 'V':
		return false
	}
	switch b[1] {
	case 'D', 'F', 'I', 'O', 'Q', 'U', 'V':
		return false
	}
	switch string(b[:2]) {
	case "BG", "GB", "NK", "KN", "TN", "NT", "ZZ":
		return false
	}
	return b[8] >= 'A' && b[8] <= 'D'
}

func BRPIS(b []byte) bool {
	if len(b) != 11 || !allDigits(b) || allSame(b) {
		return false
	}
	weights := [10]int{3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, w := range weights {
		sum += int(b[i]-'0') * w
	}
	dv := 11 - sum%11
	if dv >= 10 {
		dv = 0
	}
	return dv == int(b[10]-'0')
}
