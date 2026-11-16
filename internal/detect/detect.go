package detect

import (
	"bytes"
	"net/netip"
	"slices"

	"github.com/nyaruka/phonenumbers"

	"github.com/Guilherme-Ciano/maskwire/internal/detect/checksum"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

const (
	maxGroups      = 48
	maxCompact     = 34
	defaultBudget  = 64
	keywordWindow  = 48
	bareIDScore    = 0.6
	minPhoneDigits = 9
	maxPhoneDigits = 15
)

type Options struct {
	Regions           []string
	PhoneBudget       int
	DisablePhone      bool
	KeepReservedEmail bool
}

type Detector struct {
	regions       []string
	phoneBudget   int
	phone         bool
	reservedEmail bool
}

func New(o Options) *Detector {
	d := &Detector{
		regions:       slices.Clone(o.Regions),
		phoneBudget:   o.PhoneBudget,
		phone:         !o.DisablePhone,
		reservedEmail: !o.KeepReservedEmail,
	}
	if len(d.regions) == 0 {
		d.regions = []string{"BR", "PT", "ES", "DE", "FR", "IT"}
	}
	if d.phoneBudget <= 0 {
		d.phoneBudget = defaultBudget
	}
	return d
}

type group struct {
	start, end int
	digits     int
	letters    int
}

type scanner struct {
	d      *Detector
	text   []byte
	out    []entity.Span
	groups [maxGroups]group
	ng     int
	budget int
	v6End  int
}

func (d *Detector) PhoneBudget() int {
	return d.phoneBudget
}

func (d *Detector) Detect(dst []entity.Span, text []byte) []entity.Span {
	budget := d.phoneBudget
	return d.DetectBudget(dst, text, &budget)
}

func (d *Detector) DetectBudget(dst []entity.Span, text []byte, budget *int) []entity.Span {
	s := scanner{d: d, text: text, out: dst[:0], budget: *budget}
	s.emails()
	s.words()
	s.flushRun()
	*budget = s.budget
	return resolve(s.out)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func isLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

func isAlnum(c byte) bool {
	return isDigit(c) || isUpper(c) || isLower(c)
}

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func sepLen(b []byte, i int) int {
	c := b[i]
	switch c {
	case ' ', '.', '-', '/', '(', ')':
		return 1
	case 0xC2:
		if i+1 < len(b) && (b[i+1] == 0xA0 || b[i+1] == 0xAD) {
			return 2
		}
	case 0xE2, 0xE3, 0xEF:
		if i+2 < len(b) && wideSep(c, b[i+1], b[i+2]) {
			return 3
		}
	}
	return 0
}

func wideSep(c0, c1, c2 byte) bool {
	switch {
	case c0 == 0xE2 && c1 == 0x80:
		return c2 == 0x89 || (c2 >= 0x8B && c2 <= 0x95) || c2 == 0xAF
	case c0 == 0xE2 && c1 == 0x81:
		return c2 == 0xA0
	case c0 == 0xE2 && c1 == 0x88:
		return c2 == 0x92
	case c0 == 0xE3:
		return c1 == 0x80 && c2 == 0x80
	case c0 == 0xEF:
		return c1 == 0xBB && c2 == 0xBF
	}
	return false
}

func wideDigit(b []byte, i int) (byte, bool) {
	if i+2 < len(b) && b[i] == 0xEF && b[i+1] == 0xBC && b[i+2] >= 0x90 && b[i+2] <= 0x99 {
		return '0' + b[i+2] - 0x90, true
	}
	return 0, false
}

func appendFolded(buf []byte, k int, src []byte) int {
	for i := 0; i < len(src); i++ {
		if k >= len(buf) {
			return -1
		}
		c := src[i]
		if d, ok := wideDigit(src, i); ok {
			c = d
			i += 2
		} else if isLower(c) {
			c -= 'a' - 'A'
		}
		buf[k] = c
		k++
	}
	return k
}

func (s *scanner) words() {
	t := s.text
	i := 0
	for i < len(t) {
		c := t[i]
		if c < 0x80 && !isAlnum(c) && c != '_' {
			if c == ':' {
				s.ipv6(i)
			}
			i++
			continue
		}
		if n := sepLen(t, i); c >= 0x80 && n > 0 {
			i += n
			continue
		}
		g, clean := s.word(i)
		if clean {
			s.addGroup(g)
		} else {
			s.flushRun()
		}
		i = max(g.end, i+1)
	}
}

func (s *scanner) word(i int) (g group, clean bool) {
	t := s.text
	g.start = i
	lowers, bad := 0, false
	j := i
	for j < len(t) {
		c := t[j]
		switch {
		case isDigit(c):
			g.digits++
		case isUpper(c):
			g.letters++
		case isLower(c):
			g.letters++
			lowers++
		case c == '_':
			bad = true
		case c >= 0x80:
			if _, ok := wideDigit(t, j); ok {
				g.digits++
				j += 3
				continue
			}
			if sepLen(t, j) > 0 {
				g.end = j
				return g, !bad && (g.digits > 0 || lowers == 0)
			}
			bad = true
		default:
			g.end = j
			return g, !bad && (g.digits > 0 || lowers == 0)
		}
		j++
	}
	g.end = j
	return g, !bad && (g.digits > 0 || lowers == 0)
}

func (s *scanner) linked(prevEnd, start int) bool {
	p := prevEnd
	for range 2 {
		if p == start {
			return p != prevEnd
		}
		n := sepLen(s.text, p)
		if n == 0 {
			return false
		}
		p += n
	}
	return p == start
}

func (s *scanner) addGroup(g group) {
	if s.ng > 0 && (!s.linked(s.groups[s.ng-1].end, g.start) || s.ng == maxGroups) {
		s.flushRun()
	}
	s.groups[s.ng] = g
	s.ng++
}

func (s *scanner) flushRun() {
	if s.ng == 0 {
		return
	}
	n := s.ng
	s.ng = 0
	var claimed [maxGroups]bool
	for i := 0; i < n; i++ {
		if s.groups[i].digits == 0 && s.groups[i].letters > 0 && !ibanStart(s.text[s.groups[i].start:s.groups[i].end]) {
			if !s.mayStartAlnum(i) {
				continue
			}
		}
		end, typ := s.bestWindow(i, n)
		if end < 0 {
			continue
		}
		s.emitScore(s.groups[i].start, s.groups[end].end, typ, s.idScore(typ, i, end))
		for k := i; k <= end; k++ {
			claimed[k] = true
		}
		i = end
	}
	if s.d.phone {
		s.phones(n, &claimed)
	}
}

func (s *scanner) mayStartAlnum(i int) bool {
	g := s.groups[i]
	return g.end-g.start <= 6
}

func ibanStart(b []byte) bool {
	return len(b) >= 2 && isUpper(b[0]) && isUpper(b[1]) && checksum.IBANLength(b[:2]) > 0
}

func (s *scanner) bestWindow(i, n int) (int, entity.Type) {
	var buf [maxCompact]byte
	k := 0
	best, bestType := -1, entity.Unknown
	digitsOnly := true
	for j := i; j < n; j++ {
		g := s.groups[j]
		if k = appendFolded(buf[:], k, s.text[g.start:g.end]); k < 0 {
			break
		}
		if g.letters > 0 {
			digitsOnly = false
		}
		if j == i+3 && digitsOnly {
			if t, ok := s.ipv4(i); ok {
				return j, t
			}
		}
		if t := s.classify(buf[:k], digitsOnly, i); t != entity.Unknown {
			best, bestType = j, t
		}
	}
	return best, bestType
}

func (s *scanner) classify(b []byte, digitsOnly bool, first int) entity.Type {
	if digitsOnly {
		return s.classifyDigits(b, first)
	}
	return classifyAlnum(b)
}

func (s *scanner) classifyDigits(b []byte, first int) entity.Type {
	switch {
	case len(b) == 9 && checksum.PTNIF(b):
		return entity.PTNIF
	case len(b) == 11 && checksum.CPF(b):
		return entity.CPF
	case len(b) == 11 && checksum.DESteuerID(b) && s.context(s.groups[first].start, idWords(entity.DESteuerID)...):
		return entity.DESteuerID
	case len(b) == 14 && checksum.CNPJ(b):
		return entity.CNPJ
	case len(b) == 15 && checksum.FRNIR(b):
		return entity.FRNIR
	case checksum.CreditCard(b):
		return entity.CreditCard
	}
	return entity.Unknown
}

func classifyAlnum(b []byte) entity.Type {
	switch {
	case len(b) == 9 && checksum.ESDNI(b):
		return entity.ESDNI
	case len(b) == 9 && checksum.ESNIE(b):
		return entity.ESNIE
	case len(b) == 14 && checksum.CNPJ(b):
		return entity.CNPJ
	case len(b) == 15 && checksum.FRNIR(b):
		return entity.FRNIR
	case len(b) == 16 && checksum.ITCF(b):
		return entity.ITCF
	case checksum.IBAN(b):
		return entity.IBAN
	}
	return entity.Unknown
}

func letterByte(c byte) bool {
	return isLower(c) || isUpper(c) || c >= 0x80
}

func (s *scanner) context(pos int, words ...string) bool {
	lo := max(pos-keywordWindow, 0)
	var buf [keywordWindow]byte
	w := buf[:0]
	for _, c := range s.text[lo:pos] {
		if isUpper(c) {
			c += 'a' - 'A'
		}
		w = append(w, c)
	}
	for _, word := range words {
		for off := 0; ; {
			k := bytes.Index(w[off:], []byte(word))
			if k < 0 {
				break
			}
			at := off + k
			if at == 0 || !letterByte(w[at-1]) {
				return true
			}
			off = at + 1
		}
	}
	return false
}

func idWords(t entity.Type) []string {
	switch t {
	case entity.CPF:
		return []string{"cpf"}
	case entity.CNPJ:
		return []string{"cnpj"}
	case entity.PTNIF:
		return []string{"nif", "nipc", "contribuinte", "fiscal"}
	case entity.FRNIR:
		return []string{"nir", "insee", "sécu", "secu", "sociale"}
	case entity.CreditCard:
		return []string{"card", "cart", "tarjeta", "karte", "visa", "master", "amex", "crédit", "credit", "pan"}
	case entity.DESteuerID:
		return []string{"steuer", "idnr", "tax id", "tin"}
	case entity.Unknown, entity.Email, entity.Phone, entity.IP, entity.IPPrivate, entity.ESDNI, entity.ESNIE,
		entity.ITCF, entity.IBAN:
		return nil
	}
	return nil
}

func phoneWords() []string {
	return []string{
		"tel", "fone", "phone", "celular", "cell", "mobile", "móvil", "movil", "móvel", "movel", "whatsapp",
		"ligue", "ligar", "call", "contato", "contacto", "handy", "portable", "sms", "fax",
	}
}

func (s *scanner) idScore(t entity.Type, first, last int) float32 {
	words := idWords(t)
	if words == nil || t == entity.DESteuerID {
		return baseScore(t)
	}
	if s.context(s.groups[first].start, words...) {
		return 1
	}
	if last > first {
		return 0.9
	}
	return bareIDScore
}

func (s *scanner) ipv4(i int) (entity.Type, bool) {
	var parts [4]byte
	for k := range 4 {
		g := s.groups[i+k]
		if g.digits != g.end-g.start || g.digits > 3 {
			return entity.Unknown, false
		}
		if k > 0 && (g.start-s.groups[i+k-1].end != 1 || s.text[g.start-1] != '.') {
			return entity.Unknown, false
		}
		v := 0
		for _, c := range s.text[g.start:g.end] {
			v = v*10 + int(c-'0')
		}
		if v > 255 || (g.digits > 1 && s.text[g.start] == '0') {
			return entity.Unknown, false
		}
		parts[k] = uint8(v & 0xff)
	}
	first := s.groups[i].start
	if first > 1 && s.text[first-1] == '.' && isDigit(s.text[first-2]) {
		return entity.Unknown, false
	}
	end := s.groups[i+3].end
	if end < len(s.text)-1 && s.text[end] == '.' && isDigit(s.text[end+1]) {
		return entity.Unknown, false
	}
	addr := netip.AddrFrom4(parts)
	return ipType(addr), true
}

func ipType(a netip.Addr) entity.Type {
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() || a.IsMulticast() {
		return entity.IPPrivate
	}
	return entity.IP
}

func (s *scanner) hexLeft(colon int) (lo, colons, hex int) {
	t := s.text
	lo = colon
	for lo > 0 && (isHex(t[lo-1]) || t[lo-1] == ':') {
		lo--
		if t[lo] == ':' {
			colons++
		} else {
			hex++
		}
	}
	return lo, colons, hex
}

func (s *scanner) hexRight(colon int) (hi, colons, hex int) {
	t := s.text
	hi = colon
	for hi < len(t) && (isHex(t[hi]) || t[hi] == ':' || t[hi] == '.') {
		switch {
		case t[hi] == ':':
			colons++
		case isHex(t[hi]):
			hex++
		}
		hi++
	}
	for hi > colon && t[hi-1] == '.' {
		hi--
	}
	return hi, colons, hex
}

func wordByte(c byte) bool {
	return isAlnum(c) || c == '_'
}

func (s *scanner) ipv6(colon int) {
	if colon < s.v6End {
		return
	}
	t := s.text
	lo, lc, lh := s.hexLeft(colon)
	if lo > 0 && wordByte(t[lo-1]) {
		return
	}
	hi, rc, rh := s.hexRight(colon)
	s.v6End = hi
	if hi < len(t) && wordByte(t[hi]) {
		return
	}
	if lc+rc < 2 || lh+rh < 2 || hi-lo > 45 {
		return
	}
	addr, err := netip.ParseAddr(string(t[lo:hi]))
	if err != nil || !addr.Is6() {
		return
	}
	s.emit(lo, hi, ipType(addr))
}

func (s *scanner) phones(n int, claimed *[maxGroups]bool) {
	for i := 0; i < n; i++ {
		if claimed[i] || !s.digitGroup(i) {
			continue
		}
		j := i
		digits := 0
		for j < n && !claimed[j] && s.digitGroup(j) && digits+s.groups[j].digits <= maxPhoneDigits {
			digits += s.groups[j].digits
			j++
		}
		for end := j - 1; end >= i; end-- {
			if s.tryPhone(i, end) {
				i = end
				break
			}
		}
	}
}

func (s *scanner) digitGroup(i int) bool {
	g := s.groups[i]
	return g.letters == 0 && g.digits > 0
}

func (s *scanner) tryPhone(i, end int) bool {
	digits := 0
	for k := i; k <= end; k++ {
		digits += s.groups[k].digits
	}
	start := s.groups[i].start
	plus := start > 0 && s.text[start-1] == '+'
	paren := start > 0 && s.text[start-1] == '('
	if plus || paren {
		start--
	}
	if digits < minPhoneDigits && (!plus || digits < 7) {
		return false
	}
	if !plus && !paren && !s.context(start, phoneWords()...) {
		return false
	}
	stop := s.groups[end].end
	if s.budget <= 0 {
		if digits < minPhoneDigits {
			return false
		}
		s.emitScore(start, stop, entity.Phone, 0.5)
		return true
	}
	if !s.validPhone(string(s.text[start:stop]), plus) {
		return false
	}
	s.emit(start, stop, entity.Phone)
	return true
}

func (s *scanner) validPhone(raw string, plus bool) bool {
	regions := s.d.regions
	if plus {
		regions = []string{""}
	}
	for _, region := range regions {
		if s.budget <= 0 {
			return false
		}
		s.budget--
		num, err := phonenumbers.Parse(raw, region)
		if err == nil && phonenumbers.IsValidNumber(num) {
			return true
		}
	}
	return false
}

func (s *scanner) emit(start, end int, t entity.Type) {
	s.emitScore(start, end, t, baseScore(t))
}

func (s *scanner) emitScore(start, end int, t entity.Type, score float32) {
	s.out = append(s.out, entity.Span{Start: start, End: end, Type: t, Score: score})
}

func baseScore(t entity.Type) float32 {
	switch t {
	case entity.Email:
		return 0.95
	case entity.Phone:
		return 0.85
	case entity.IP, entity.IPPrivate:
		return 0.9
	case entity.CPF, entity.CNPJ, entity.PTNIF, entity.ESDNI, entity.ESNIE, entity.DESteuerID,
		entity.FRNIR, entity.ITCF, entity.IBAN, entity.CreditCard:
		return 1
	case entity.Unknown:
		return 0
	}
	return 0
}

func emailLocal(c byte) bool {
	return isAlnum(c) || c == '.' || c == '_' || c == '%' || c == '+' || c == '-' || c >= 0x80
}

func emailDomain(c byte) bool {
	return isAlnum(c) || c == '.' || c == '-'
}

func (s *scanner) emails() {
	t := s.text
	off := 0
	for {
		k := bytes.IndexByte(t[off:], '@')
		if k < 0 {
			return
		}
		at := off + k
		off = at + 1
		lo, hi, ok := s.emailAt(at)
		if !ok {
			continue
		}
		s.emit(lo, hi, entity.Email)
		off = hi
	}
}

func (s *scanner) emailAt(at int) (lo, hi int, ok bool) {
	t := s.text
	lo = at
	for lo > 0 && emailLocal(t[lo-1]) {
		lo--
	}
	for lo < at && (t[lo] == '.' || t[lo] == '-' || t[lo] >= 0x80 && sepLen(t, lo) > 0) {
		lo += max(sepLen(t, lo), 1)
	}
	hi = at + 1
	for hi < len(t) && emailDomain(t[hi]) {
		hi++
	}
	for hi > at+1 && (t[hi-1] == '.' || t[hi-1] == '-') {
		hi--
	}
	domain := t[at+1 : hi]
	if lo == at || !validDomain(domain) || (s.d.reservedEmail && reservedDomain(domain)) {
		return 0, 0, false
	}
	return lo, hi, true
}

func validDomain(d []byte) bool {
	dot := bytes.LastIndexByte(d, '.')
	if dot <= 0 || len(d)-dot <= 2 || bytes.Contains(d, []byte("..")) {
		return false
	}
	for _, c := range d[dot+1:] {
		if !isUpper(c) && !isLower(c) {
			return false
		}
	}
	return true
}

func hasSuffixFold(b []byte, suffix string) bool {
	if len(b) < len(suffix) {
		return false
	}
	tail := b[len(b)-len(suffix):]
	for i := range len(suffix) {
		c := tail[i]
		if isUpper(c) {
			c += 'a' - 'A'
		}
		if c != suffix[i] {
			return false
		}
	}
	return len(b) == len(suffix) || b[len(b)-len(suffix)-1] == '.' || suffix[0] == '.'
}

func reservedDomain(d []byte) bool {
	for _, r := range [...]string{"example.com", "example.org", "example.net", ".example", ".invalid", ".test", ".localhost"} {
		if hasSuffixFold(d, r) {
			return true
		}
	}
	return false
}

func better(a, b entity.Span) bool {
	pa, pb := entity.Priority(a.Type), entity.Priority(b.Type)
	if pa != pb {
		return pa > pb
	}
	return a.Len() > b.Len()
}

func resolve(spans []entity.Span) []entity.Span {
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].Start < spans[j-1].Start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
	out := spans[:0]
	for _, c := range spans {
		keep := true
		for len(out) > 0 && out[len(out)-1].Overlaps(c) {
			if better(c, out[len(out)-1]) {
				out = out[:len(out)-1]
				continue
			}
			keep = false
			break
		}
		if keep {
			out = append(out, c)
		}
	}
	return out
}
