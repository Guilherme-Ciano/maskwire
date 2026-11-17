package detect

import "bytes"

const (
	secretScore    = 0.95
	minSecretValue = 12
	maxSecretValue = 512
	pemBegin       = "-----BEGIN "
	pemEnd         = "-----END "
	pemDashes      = "-----"
)

type charset uint8

const (
	setUpperDigit charset = iota
	setAlnum
	setAlnumUnderscore
	setAlnumDash
	setURLSafe
	setLetters
)

func (c charset) has(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return c != setLetters
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return c != setUpperDigit
	case b == '_':
		return c == setAlnumUnderscore || c == setURLSafe
	case b == '-':
		return c == setAlnumDash || c == setURLSafe
	}
	return false
}

type prefixed struct {
	prefix string
	set    charset
	min    int
	max    int
}

func secretPrefixes() []prefixed {
	return []prefixed{
		{"AKIA", setUpperDigit, 16, 16},
		{"ASIA", setUpperDigit, 16, 16},
		{"ABIA", setUpperDigit, 16, 16},
		{"ACCA", setUpperDigit, 16, 16},
		{"AIza", setURLSafe, 35, 35},
		{"ghp_", setAlnum, 36, 255},
		{"gho_", setAlnum, 36, 255},
		{"ghu_", setAlnum, 36, 255},
		{"ghs_", setAlnum, 36, 255},
		{"ghr_", setAlnum, 36, 255},
		{"github_pat_", setAlnumUnderscore, 22, 255},
		{"glpat-", setURLSafe, 20, 255},
		{"sk_live_", setAlnum, 16, 255},
		{"rk_live_", setAlnum, 16, 255},
		{"sk_test_", setAlnum, 16, 255},
		{"rk_test_", setAlnum, 16, 255},
		{"sk-ant-", setURLSafe, 32, 255},
		{"sk-proj-", setURLSafe, 32, 255},
		{"sk-", setAlnum, 32, 255},
		{"xoxb-", setAlnumDash, 10, 255},
		{"xoxp-", setAlnumDash, 10, 255},
		{"xoxa-", setAlnumDash, 10, 255},
		{"xoxr-", setAlnumDash, 10, 255},
		{"xoxs-", setAlnumDash, 10, 255},
		{"npm_", setAlnum, 36, 36},
		{"hf_", setLetters, 30, 40},
	}
}

func assignmentWords() []string {
	return []string{
		"password", "passwd", "pwd", "secret", "client_secret", "api_key", "apikey", "api-key", "access_token",
		"auth_token", "token", "private_key", "secret_key", "aws_secret_access_key",
	}
}

type secretIndex struct {
	first   [256]bool
	byFirst [256][]prefixed
	assign  [256][][]byte
}

func newSecretIndex() *secretIndex {
	ix := &secretIndex{}
	for _, p := range secretPrefixes() {
		c := p.prefix[0]
		ix.first[c] = true
		ix.byFirst[c] = append(ix.byFirst[c], p)
	}
	for _, w := range assignmentWords() {
		for _, c := range []byte{w[0], w[0] - ('a' - 'A')} {
			ix.assign[c] = append(ix.assign[c], []byte(w))
		}
	}
	ix.first['e'] = true
	return ix
}

func spanOf(t []byte, i int, set charset) int {
	for i < len(t) && set.has(t[i]) {
		i++
	}
	return i
}

func hasPrefix(t []byte, i int, prefix string) bool {
	return len(t)-i >= len(prefix) && string(t[i:i+len(prefix)]) == prefix
}

func (s *scanner) secretAt(i int) (start, end int) {
	t := s.text
	ix := s.d.secrets
	if i > 0 && (isAlnum(t[i-1]) || t[i-1] == '_' || t[i-1] == '-') {
		return -1, -1
	}
	for _, p := range ix.byFirst[t[i]] {
		if !hasPrefix(t, i, p.prefix) {
			continue
		}
		body := i + len(p.prefix)
		end := spanOf(t, body, p.set)
		if n := end - body; n >= p.min && n <= p.max {
			return i, end
		}
	}
	if t[i] == 'e' {
		if end := jwt(t, i); end > 0 {
			return i, end
		}
	}
	return -1, -1
}

func jwt(t []byte, i int) int {
	if !hasPrefix(t, i, "eyJ") {
		return -1
	}
	end := i
	for part := range 3 {
		next := spanOf(t, end, setURLSafe)
		if next-end < 4 {
			return -1
		}
		end = next
		if part < 2 {
			if end >= len(t) || t[end] != '.' {
				return -1
			}
			end++
		}
	}
	return end
}

func quoteOrSpace(c byte) bool {
	return c == '"' || c == '\'' || c == ' '
}

func (s *scanner) assignment(p int) (start, end int) {
	t := s.text
	e := p
	for e > 0 && quoteOrSpace(t[e-1]) {
		e--
	}
	w := e
	for w > 0 && (isAlnum(t[w-1]) || t[w-1] == '_' || t[w-1] == '-') {
		w--
	}
	if w == e || len(s.d.secrets.assign[t[w]]) == 0 {
		return -1, -1
	}
	key := t[w:e]
	found := false
	for _, word := range s.d.secrets.assign[t[w]] {
		if bytes.EqualFold(key, word) {
			found = true
			break
		}
	}
	if !found {
		return -1, -1
	}
	j := p + 1
	for j < len(t) && quoteOrSpace(t[j]) {
		j++
	}
	stop := j
	for stop < len(t) && stop-j < maxSecretValue && t[stop] > ' ' && t[stop] != '"' && t[stop] != '\'' && t[stop] != ',' {
		stop++
	}
	if !machineMade(t[j:stop]) {
		return -1, -1
	}
	return j, stop
}

func machineMade(v []byte) bool {
	if len(v) < minSecretValue {
		return false
	}
	var lower, upper, digit, other bool
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= '0' && c <= '9':
			digit = true
		default:
			other = true
		}
	}
	classes := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			classes++
		}
	}
	return classes >= 3 || (classes == 2 && digit)
}

func (s *scanner) pemBlock(i int) int {
	t := s.text
	if !hasPrefix(t, i, pemBegin) {
		return -1
	}
	head := bytes.Index(t[i+len(pemBegin):], []byte(pemDashes))
	if head < 0 || !bytes.Contains(t[i:i+len(pemBegin)+head], []byte("PRIVATE KEY")) {
		return -1
	}
	tail := bytes.Index(t[i:], []byte(pemEnd))
	if tail < 0 {
		return -1
	}
	stop := bytes.Index(t[i+tail+len(pemEnd):], []byte(pemDashes))
	if stop < 0 {
		return -1
	}
	return i + tail + len(pemEnd) + stop + len(pemDashes)
}
