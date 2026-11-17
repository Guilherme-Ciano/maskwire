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
	maxGroups        = 48
	maxCompact       = 34
	defaultBudget    = 64
	keywordWindow    = 48
	bareIDScore      = 0.6
	bareSteuerScore  = 0.5
	formattedScore   = 0.9
	maxPrefixKeyword = 4
	maxLowerGroup    = 4
	minPhoneDigits   = 9
	maxPhoneDigits   = 15
)

type Options struct {
	Regions           []string
	PhoneBudget       int
	DisablePhone      bool
	KeepReservedEmail bool
	Names             bool
	DisableSecrets    bool
}

type Detector struct {
	regions       []string
	phoneBudget   int
	phone         bool
	reservedEmail bool
	names         *gazetteer
	rules         *ruleIndex
	phoneWords    [][]byte
	secrets       *secretIndex
}

func New(o Options) *Detector {
	d := &Detector{
		regions:       slices.Clone(o.Regions),
		phoneBudget:   o.PhoneBudget,
		phone:         !o.DisablePhone,
		reservedEmail: !o.KeepReservedEmail,
	}
	d.rules = newRuleIndex()
	d.phoneWords = toBytes(phoneWords())
	if !o.DisableSecrets {
		d.secrets = newSecretIndex()
	}
	if o.Names {
		d.names = newGazetteer()
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
	d          *Detector
	text       []byte
	out        []entity.Span
	groups     [maxGroups]group
	ng         int
	run        int
	hint       [maxHint]byte
	hintLen    int
	negative   bool
	phoneState PhoneState
	v6End      int
}

type PhoneState struct {
	Budget int
	seen   map[string]bool
}

func (d *Detector) NewPhoneState() *PhoneState {
	return &PhoneState{Budget: d.phoneBudget}
}

func (d *Detector) Detect(dst []entity.Span, text []byte) []entity.Span {
	st := PhoneState{Budget: d.phoneBudget}
	return d.DetectWith(dst, text, &st)
}

func (d *Detector) DetectWith(dst []entity.Span, text []byte, st *PhoneState) []entity.Span {
	return d.DetectField(dst, text, nil, st)
}

func (d *Detector) DetectField(dst []entity.Span, text, field []byte, st *PhoneState) []entity.Span {
	s := scanner{d: d, text: text, out: dst[:0], phoneState: *st}
	if len(field) > 0 {
		s.hintLen = normalizeField(&s.hint, field)
		s.negative = negativeField(&s.hint, s.hintLen)
	}
	s.emails()
	s.obfuscatedEmails()
	s.words()
	s.flushRun()
	if d.names != nil {
		s.names()
	}
	*st = s.phoneState
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
	for i := 0; i < len(t); {
		c := t[i]
		if c < 0x80 && !isAlnum(c) && c != '_' {
			if c == ':' || c == '=' || c == '-' {
				i = s.punct(i)
				continue
			}
			i++
			continue
		}
		if c >= 0x80 {
			if n := sepLen(t, i); n > 0 {
				i += n
				continue
			}
		}
		i = s.token(i)
	}
}

func (s *scanner) punct(i int) int {
	c := s.text[i]
	if c == ':' {
		s.ipv6(i)
	}
	if s.d.secrets != nil && (c == ':' || c == '=' || c == '-') {
		if end := s.punctSecret(i); end > 0 {
			return end
		}
	}
	return i + 1
}

func (s *scanner) token(i int) int {
	c := s.text[i]
	if isLower(c) {
		if j := s.lowerWord(i); j > 0 {
			return j
		}
	}
	if s.d.secrets != nil && s.d.secrets.first[c] {
		if end := s.secret(i); end > 0 {
			return end
		}
	}
	g, clean := s.word(i)
	if clean {
		s.addGroup(g)
	} else {
		s.flushRun()
	}
	return max(g.end, i+1)
}

func (s *scanner) secret(i int) int {
	start, end := s.secretAt(i)
	return s.emitSecret(start, end)
}

func (s *scanner) emitSecret(start, end int) int {
	if start < 0 || end <= start {
		return -1
	}
	s.flushRun()
	s.emitScore(start, end, entity.Secret, secretScore)
	return end
}

func (s *scanner) punctSecret(i int) int {
	if s.text[i] != '-' {
		return s.emitSecret(s.assignment(i))
	}
	if end := s.pemBlock(i); end > 0 {
		return s.emitSecret(i, end)
	}
	q := i
	for q > 0 && isAlnum(s.text[q-1]) {
		q--
	}
	if q == i || !s.d.secrets.first[s.text[q]] {
		return -1
	}
	return s.emitSecret(s.secretAt(q))
}

func (s *scanner) lowerWord(i int) int {
	j := lowerRun(s.text, i)
	if j < 0 {
		return -1
	}
	if j-i <= maxLowerGroup && s.ng > 0 && s.linked(s.groups[s.ng-1].end, i) {
		s.addGroup(group{start: i, end: j, letters: j - i})
	} else {
		s.flushRun()
	}
	return j
}

func lowerRun(t []byte, i int) int {
	j := i + 1
	for j < len(t) && isLower(t[j]) {
		j++
	}
	if j < len(t) {
		c := t[j]
		if c >= 0x80 || isAlnum(c) || c == '_' {
			return -1
		}
	}
	return j
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
	s.run = n
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
		score, ok := s.accept(typ, i, end, n, &claimed)
		if !ok {
			continue
		}
		s.emitScore(s.groups[i].start, s.groups[end].end, typ, score)
		for k := i; k <= end; k++ {
			claimed[k] = true
		}
		i = end
	}
	if s.d.phone {
		s.phones(n, &claimed)
	}
}

func (s *scanner) accept(typ entity.Type, i, end, n int, claimed *[maxGroups]bool) (float32, bool) {
	score := s.idScore(typ, i, end)
	switch {
	case score < formattedScore && s.phoneShaped(i):
		return 0, false
	case score < 1 && (s.phonePrefixed(0) || s.splitsToken(i, end, n, claimed) || s.looseGroups(typ, i, end)):
		return 0, false
	case s.negative && score < formattedScore:
		return 0, false
	}
	return score, true
}

func (s *scanner) phoneShaped(i int) bool {
	start := s.groups[i].start
	if start > 0 && (s.text[start-1] == '+' || s.text[start-1] == '(') {
		return true
	}
	return s.phoneContext(start)
}

func (s *scanner) phonePrefixed(i int) bool {
	start := s.groups[i].start
	return start > 0 && (s.text[start-1] == '+' || s.text[start-1] == '(')
}

func (s *scanner) tight(from, to int) bool {
	for _, c := range s.text[from:to] {
		if c != ' ' {
			return true
		}
	}
	return false
}

func (s *scanner) splitsToken(first, last, n int, claimed *[maxGroups]bool) bool {
	if first > 0 && !claimed[first-1] && s.tight(s.groups[first-1].end, s.groups[first].start) {
		return true
	}
	return last < n-1 && s.tight(s.groups[last].end, s.groups[last+1].start)
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
	ctx := lowered{pos: s.groups[i].start}
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
		if t := s.classify(buf[:k], digitsOnly, i, j, &ctx); t != entity.Unknown {
			best, bestType = j, t
		}
	}
	return best, bestType
}

func hetuCentury(b []byte) bool {
	if len(b) != 11 {
		return false
	}
	switch b[6] {
	case 'A', 'B', 'C', 'D', 'E', 'F', 'U', 'V', 'W', 'X', 'Y':
	default:
		return false
	}
	var buf [10]byte
	copy(buf[:6], b[:6])
	copy(buf[6:], b[7:])
	return checksum.FIHETU(buf[:])
}

func letterByte(c byte) bool {
	return isLower(c) || isUpper(c) || c >= 0x80
}

type lowered struct {
	buf  [keywordWindow]byte
	n    int
	pos  int
	done bool
}

func (s *scanner) lower(pos int) lowered {
	var l lowered
	from := max(pos-keywordWindow, 0)
	if room := keywordWindow - (pos - from); room > 0 && s.hintLen > 0 {
		l.n = copy(l.buf[:], s.hint[max(s.hintLen-room, 0):s.hintLen])
	}
	for _, c := range s.text[from:pos] {
		if isUpper(c) {
			c += 'a' - 'A'
		}
		l.buf[l.n] = c
		l.n++
	}
	return l
}

func (s *scanner) lazy(l *lowered) *lowered {
	if !l.done {
		*l = s.lower(l.pos)
		l.done = true
	}
	return l
}

func (l *lowered) has(strict bool, words [][]byte) bool {
	w := l.buf[:l.n]
	for _, word := range words {
		for off := 0; off < len(w); {
			k := bytes.Index(w[off:], word)
			if k < 0 {
				break
			}
			at := off + k
			end := at + len(word)
			short := strict && len(word) <= maxPrefixKeyword
			if (at == 0 || !letterByte(w[at-1])) && (!short || end >= len(w) || !letterByte(w[end])) {
				return true
			}
			off = at + 1
		}
	}
	return false
}

func (s *scanner) keyword(pos int, words [][]byte) bool {
	l := s.lower(pos)
	return l.has(true, words)
}

func (s *scanner) phoneContext(pos int) bool {
	l := s.lower(pos)
	return l.has(false, s.d.phoneWords)
}

func toBytes(words []string) [][]byte {
	if words == nil {
		return nil
	}
	out := make([][]byte, len(words))
	for i, w := range words {
		out[i] = []byte(w)
	}
	return out
}

func phoneWords() []string {
	return []string{
		"tel", "fone", "phone", "celular", "cell", "mobile", "móvil", "movil", "móvel", "movel", "whatsapp",
		"ligue", "ligar", "call", "contato", "contacto", "handy", "portable", "sms", "fax",
	}
}

func (s *scanner) sizes(first, last int, lens ...int) bool {
	if last-first+1 != len(lens) {
		return false
	}
	for k, n := range lens {
		g := s.groups[first+k]
		if g.digits+g.letters != n {
			return false
		}
	}
	return true
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
	if !plus && !paren && !s.phoneContext(start) {
		return false
	}
	stop := s.groups[end].end
	raw := string(s.text[start:stop])
	valid, cached := s.phoneState.seen[raw]
	switch {
	case cached:
	case s.phoneState.Budget <= 0:
		if digits < minPhoneDigits {
			return false
		}
		s.emitScore(start, stop, entity.Phone, 0.5)
		return true
	default:
		valid = s.validPhone(raw, plus)
	}
	if !valid {
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
	valid := false
	for _, region := range regions {
		if s.phoneState.Budget <= 0 {
			return false
		}
		s.phoneState.Budget--
		num, err := phonenumbers.Parse(raw, region)
		if err == nil && phonenumbers.IsValidNumber(num) {
			valid = true
			break
		}
	}
	if s.phoneState.seen == nil {
		s.phoneState.seen = map[string]bool{}
	}
	s.phoneState.seen[raw] = valid
	return valid
}

func (s *scanner) emit(start, end int, t entity.Type) {
	s.emitScore(start, end, t, baseScore(t))
}

func (s *scanner) emitScore(start, end int, t entity.Type, score float32) {
	s.out = append(s.out, entity.Span{Start: start, End: end, Type: t, Score: score})
}

func baseScore(t entity.Type) float32 {
	if !t.Valid() {
		return 0
	}
	if t == entity.Email {
		return 0.95
	}
	if t == entity.Phone || t == entity.Person {
		return 0.85
	}
	if t == entity.IP || t == entity.IPPrivate {
		return 0.9
	}
	return 1
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

func markerAt(t []byte, i int, word string) int {
	if i >= len(t) || (t[i] != '(' && t[i] != '[') {
		return 0
	}
	n := len(word) + 2
	if i+n > len(t) {
		return 0
	}
	closer := byte(')')
	if t[i] == '[' {
		closer = ']'
	}
	for k := range len(word) {
		c := t[i+1+k]
		if isUpper(c) {
			c += 'a' - 'A'
		}
		if c != word[k] {
			return 0
		}
	}
	if t[i+n-1] != closer {
		return 0
	}
	return n
}

func skipSpaces(t []byte, i, dir int) int {
	for i >= 0 && i < len(t) && t[i] == ' ' {
		i += dir
	}
	return i
}

func (s *scanner) obfuscatedEmails() {
	for _, closer := range [...]byte{')', ']'} {
		s.obfuscatedWith(closer)
	}
}

func (s *scanner) obfuscatedWith(closer byte) {
	t := s.text
	for off := 0; off < len(t); {
		c := bytes.IndexByte(t[off:], closer)
		if c < 0 {
			return
		}
		k := off + c - 3
		off += c + 1
		if k < 0 || markerAt(t, k, "at") == 0 {
			continue
		}
		hi, ok := s.obfuscatedDomain(skipSpaces(t, k+4, 1))
		lo := skipSpaces(t, k-1, -1) + 1
		end := lo
		for lo > 0 && emailLocal(t[lo-1]) && t[lo-1] < 0x80 {
			lo--
		}
		if !ok || lo == end {
			continue
		}
		s.emitScore(lo, hi, entity.Email, 0.9)
		off = max(off, hi)
	}
}

func (s *scanner) obfuscatedDomain(i int) (end int, ok bool) {
	t := s.text
	var labels, last int
	for {
		start := i
		for i < len(t) && (isAlnum(t[i]) || t[i] == '-') {
			i++
		}
		if i == start {
			return 0, false
		}
		labels++
		last = i - start
		end = i
		j := skipSpaces(t, i, 1)
		if n := markerAt(t, j, "dot"); n > 0 {
			i = skipSpaces(t, j+n, 1)
			continue
		}
		if i < len(t) && t[i] == '.' && i+1 < len(t) && isAlnum(t[i+1]) {
			i++
			continue
		}
		break
	}
	return end, labels >= 2 && last >= 2
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
	slices.SortStableFunc(spans, func(a, b entity.Span) int { return a.Start - b.Start })
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
