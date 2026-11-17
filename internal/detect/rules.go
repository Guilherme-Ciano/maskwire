package detect

import (
	"github.com/Guilherme-Ciano/maskwire/internal/detect/checksum"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type need uint8

const (
	needKeyword need = iota
	needLayout
	needNothing
)

type window uint8

const (
	digitWindow window = 1 << iota
	alnumWindow
	anyWindow = digitWindow | alnumWindow
)

type check uint8

const (
	checkCLRUT check = iota
	checkUYCI
	checkAUTFN
	checkNZIRD
	checkNLBSN
	checkUSSSN
	checkUSITIN
	checkUSEIN
	checkUSABA
	checkILID
	checkPTNIF
	checkATSVNR
	checkSEPNR
	checkFIHETU
	checkCONIT
	checkPLPESEL
	checkBENN
	checkBRPIS
	checkARCUIT
	checkPERUC
	checkTRTCKN
	checkCPF
	checkDESteuerID
	checkAUABN
	checkINAadhaar
	checkJPMyNumber
	checkKRRRN
	checkZAID
	checkIEPPSN
	checkESDNI
	checkESNIE
	checkUKNINO
	checkSGNRIC
	checkINPAN
	checkHetuCentury
	checkMXRFC
	checkCNPJ
	checkFRNIR
	checkITCF
	checkMXCURP
	checkCNRIC
	checkCreditCard
	checkIBAN
)

func (c check) valid(b []byte) bool {
	switch c {
	case checkCLRUT:
		return checksum.CLRUT(b)
	case checkUYCI:
		return checksum.UYCI(b)
	case checkAUTFN:
		return checksum.AUTFN(b)
	case checkNZIRD:
		return checksum.NZIRD(b)
	case checkNLBSN:
		return checksum.NLBSN(b)
	case checkUSSSN:
		return checksum.USSSN(b)
	case checkUSITIN:
		return checksum.USITIN(b)
	case checkUSEIN:
		return checksum.USEIN(b)
	case checkUSABA:
		return checksum.USABA(b)
	case checkILID:
		return checksum.ILID(b)
	case checkPTNIF:
		return checksum.PTNIF(b)
	case checkATSVNR:
		return checksum.ATSVNR(b)
	case checkSEPNR:
		return checksum.SEPNR(b)
	case checkFIHETU:
		return checksum.FIHETU(b)
	case checkCONIT:
		return checksum.CONIT(b)
	case checkPLPESEL:
		return checksum.PLPESEL(b)
	case checkBENN:
		return checksum.BENN(b)
	case checkBRPIS:
		return checksum.BRPIS(b)
	case checkARCUIT:
		return checksum.ARCUIT(b)
	case checkPERUC:
		return checksum.PERUC(b)
	case checkTRTCKN:
		return checksum.TRTCKN(b)
	case checkCPF:
		return checksum.CPF(b)
	case checkDESteuerID:
		return checksum.DESteuerID(b)
	case checkAUABN:
		return checksum.AUABN(b)
	case checkINAadhaar:
		return checksum.INAadhaar(b)
	case checkJPMyNumber:
		return checksum.JPMyNumber(b)
	case checkKRRRN:
		return checksum.KRRRN(b)
	case checkZAID:
		return checksum.ZAID(b)
	case checkIEPPSN:
		return checksum.IEPPSN(b)
	case checkESDNI:
		return checksum.ESDNI(b)
	case checkESNIE:
		return checksum.ESNIE(b)
	case checkUKNINO:
		return checksum.UKNINO(b)
	case checkSGNRIC:
		return checksum.SGNRIC(b)
	case checkINPAN:
		return checksum.INPAN(b)
	case checkHetuCentury:
		return hetuCentury(b)
	case checkMXRFC:
		return checksum.MXRFC(b)
	case checkCNPJ:
		return checksum.CNPJ(b)
	case checkFRNIR:
		return checksum.FRNIR(b)
	case checkITCF:
		return checksum.ITCF(b)
	case checkMXCURP:
		return checksum.MXCURP(b)
	case checkCNRIC:
		return checksum.CNRIC(b)
	case checkCreditCard:
		return checksum.CreditCard(b)
	case checkIBAN:
		return checksum.IBAN(b)
	}
	return false
}

type rule struct {
	typ      entity.Type
	lens     []int
	windows  window
	check    check
	need     need
	words    []string
	layouts  [][]int
	distinct bool
	bare     float32
	loose    bool
	single   bool
	keys     [][]byte
}

func span(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		out = append(out, n)
	}
	return out
}

func ruleSet() []rule {
	cardWords := []string{
		"card",
		"cartão",
		"cartao",
		"carte",
		"tarjeta",
		"karte",
		"visa",
		"master",
		"amex",
		"crédit",
		"credit",
		"pan",
	}
	cardLayouts := [][]int{{4, 4, 4, 4}, {4, 6, 5}, {4, 4, 4, 4, 3}}
	return []rule{
		{
			typ: entity.CLRUT, lens: []int{8, 9}, windows: anyWindow, check: checkCLRUT, need: needLayout,
			words: []string{"rut", "run", "rol único"}, layouts: [][]int{{1, 3, 3, 1}, {2, 3, 3, 1}},
		},
		{
			typ: entity.UYCI, lens: []int{8}, windows: digitWindow, check: checkUYCI, need: needLayout,
			words: []string{"cédula", "cedula", "c.i.", "ci "}, layouts: [][]int{{1, 3, 3, 1}},
		},
		{
			typ: entity.AUTFN, lens: []int{8, 9}, windows: digitWindow, check: checkAUTFN, need: needKeyword,
			words: []string{"tfn", "tax file"},
		},
		{
			typ: entity.NZIRD, lens: []int{8, 9}, windows: digitWindow, check: checkNZIRD, need: needKeyword,
			words: []string{"ird", "inland revenue"},
		},
		{
			typ: entity.NLBSN, lens: []int{9}, windows: digitWindow, check: checkNLBSN, need: needKeyword,
			words: []string{"bsn", "burgerservice", "sofi"}, distinct: true,
		},
		{
			typ: entity.USSSN, lens: []int{9}, windows: digitWindow, check: checkUSSSN, need: needLayout,
			words: []string{"ssn", "social security", "soc. sec"}, layouts: [][]int{{3, 2, 4}},
		},
		{
			typ: entity.USITIN, lens: []int{9}, windows: digitWindow, check: checkUSITIN, need: needLayout,
			words: []string{"itin", "taxpayer identification"}, layouts: [][]int{{3, 2, 4}},
		},
		{
			typ: entity.USEIN, lens: []int{9}, windows: digitWindow, check: checkUSEIN, need: needKeyword,
			words: []string{"ein", "employer identification", "fein"},
		},
		{
			typ: entity.USABA, lens: []int{9}, windows: digitWindow, check: checkUSABA, need: needKeyword,
			words: []string{"routing", "aba", "rtn", "transit number"},
		},
		{
			typ: entity.ILID, lens: []int{9}, windows: digitWindow, check: checkILID, need: needKeyword,
			words: []string{"teudat", "zehut", "ת.ז", "תעודת זהות", "israeli id"},
		},
		{
			typ: entity.PTNIF, lens: []int{9}, windows: digitWindow, check: checkPTNIF, need: needNothing,
			words: []string{"nif", "nipc", "contribuinte", "fiscal"}, layouts: [][]int{{3, 3, 3}},
		},
		{
			typ: entity.ATSVNR, lens: []int{10}, windows: digitWindow, check: checkATSVNR, need: needKeyword,
			words: []string{"svnr", "sv-nr", "sv nr", "sozialversicherung", "versicherungsnummer"}, distinct: true,
		},
		{
			typ: entity.SEPNR, lens: []int{10, 12}, windows: digitWindow, check: checkSEPNR, need: needLayout,
			words: []string{"personnummer", "pnr", "personal identity number"}, layouts: [][]int{{6, 4}, {8, 4}},
			distinct: true,
		},
		{
			typ: entity.FIHETU, lens: []int{10}, windows: digitWindow, check: checkFIHETU, need: needLayout,
			words: hetuWords(), layouts: [][]int{{6, 4}}, distinct: true,
		},
		{
			typ: entity.CONIT, lens: []int{10}, windows: digitWindow, check: checkCONIT, need: needLayout,
			words: []string{"nit", "dian"}, layouts: [][]int{{3, 3, 3, 1}},
		},
		{
			typ: entity.PLPESEL, lens: []int{11}, windows: digitWindow, check: checkPLPESEL, need: needKeyword,
			words: []string{"pesel"}, distinct: true,
		},
		{
			typ: entity.BENN, lens: []int{11}, windows: digitWindow, check: checkBENN, need: needLayout,
			words:   []string{"rijksregister", "registre national", "insz", "niss", "national number", "nn"},
			layouts: [][]int{{2, 2, 2, 3, 2}}, distinct: true,
		},
		{
			typ: entity.BRPIS, lens: []int{11}, windows: digitWindow, check: checkBRPIS, need: needLayout,
			words: []string{"pis", "pasep", "nis", "nit"}, layouts: [][]int{{3, 5, 2, 1}}, distinct: true,
		},
		{
			typ: entity.ARCUIT, lens: []int{11}, windows: digitWindow, check: checkARCUIT, need: needLayout,
			words: []string{"cuit", "cuil", "cdi"}, layouts: [][]int{{2, 8, 1}},
		},
		{
			typ: entity.PERUC, lens: []int{11}, windows: digitWindow, check: checkPERUC, need: needKeyword,
			words: []string{"ruc", "sunat"},
		},
		{
			typ: entity.TRTCKN, lens: []int{11}, windows: digitWindow, check: checkTRTCKN, need: needKeyword,
			words: []string{"tckn", "kimlik", "t.c. no", "tc no"},
		},
		{
			typ: entity.CPF, lens: []int{11}, windows: digitWindow, check: checkCPF, need: needNothing,
			words: []string{"cpf"}, layouts: [][]int{{3, 3, 3, 2}},
		},
		{
			typ:     entity.DESteuerID,
			lens:    []int{11},
			windows: digitWindow,
			check:   checkDESteuerID,
			need:    needNothing,
			words:   []string{"steuer", "idnr", "tax id", "tin"},
			layouts: [][]int{{2, 3, 3, 3}},
			bare:    bareSteuerScore,
		},
		{
			typ: entity.AUABN, lens: []int{11}, windows: digitWindow, check: checkAUABN, need: needLayout,
			words: []string{"abn", "australian business"}, layouts: [][]int{{2, 3, 3, 3}},
		},
		{
			typ: entity.INAadhaar, lens: []int{12}, windows: digitWindow, check: checkINAadhaar, need: needLayout,
			words: []string{"aadhaar", "aadhar", "uidai", "आधार"}, layouts: [][]int{{4, 4, 4}},
		},
		{
			typ:     entity.JPMyNumber,
			lens:    []int{12},
			windows: digitWindow,
			check:   checkJPMyNumber,
			need:    needKeyword,
			words:   []string{"my number", "mynumber", "マイナンバー", "個人番号"},
		},
		{
			typ: entity.KRRRN, lens: []int{13}, windows: digitWindow, check: checkKRRRN, need: needLayout,
			words: []string{"rrn", "주민등록번호", "resident registration"}, layouts: [][]int{{6, 7}}, distinct: true,
		},
		{
			typ: entity.ZAID, lens: []int{13}, windows: digitWindow, check: checkZAID, need: needKeyword,
			words: []string{"id number", "identity number", "rsa id", "sa id"},
		},
		{
			typ: entity.IEPPSN, lens: []int{8, 9}, windows: alnumWindow, check: checkIEPPSN, need: needNothing,
			words: []string{"ppsn", "pps"}, distinct: true,
		},
		{
			typ:     entity.ESDNI,
			lens:    []int{9},
			windows: alnumWindow,
			check:   checkESDNI,
			need:    needNothing,
			loose:   true,
		},
		{
			typ:     entity.ESNIE,
			lens:    []int{9},
			windows: alnumWindow,
			check:   checkESNIE,
			need:    needNothing,
			loose:   true,
		},
		{
			typ: entity.UKNINO, lens: []int{9}, windows: alnumWindow, check: checkUKNINO, need: needNothing,
			words: []string{"nino", "national insurance", "ni number"}, distinct: true,
		},
		{
			typ: entity.SGNRIC, lens: []int{9}, windows: alnumWindow, check: checkSGNRIC, need: needNothing,
			words: []string{"nric", "fin"}, distinct: true,
		},
		{
			typ: entity.FIHETU, lens: []int{10}, windows: alnumWindow, check: checkFIHETU, need: needNothing,
			words: hetuWords(), distinct: true,
		},
		{
			typ: entity.INPAN, lens: []int{10}, windows: alnumWindow, check: checkINPAN, need: needNothing,
			words: []string{"pan", "permanent account"},
		},
		{
			typ: entity.FIHETU, lens: []int{11}, windows: alnumWindow, check: checkHetuCentury, need: needNothing,
			words: hetuWords(), distinct: true,
		},
		{
			typ: entity.MXRFC, lens: []int{12, 13}, windows: alnumWindow, check: checkMXRFC, need: needNothing,
			words: []string{"rfc"}, distinct: true, single: true,
		},
		{
			typ: entity.CNPJ, lens: []int{14}, windows: anyWindow, check: checkCNPJ, need: needNothing,
			words: []string{"cnpj"}, layouts: [][]int{{2, 3, 3, 4, 2}},
		},
		{
			typ: entity.FRNIR, lens: []int{15}, windows: anyWindow, check: checkFRNIR, need: needNothing,
			words: []string{"nir", "insee", "sécu", "securite", "sociale"}, layouts: [][]int{{1, 2, 2, 2, 3, 3, 2}},
		},
		{typ: entity.ITCF, lens: []int{16}, windows: alnumWindow, check: checkITCF, need: needNothing, loose: true},
		{
			typ: entity.MXCURP, lens: []int{18}, windows: alnumWindow, check: checkMXCURP, need: needNothing,
			words: []string{"curp"}, distinct: true, single: true,
		},
		{
			typ: entity.CNRIC, lens: []int{18}, windows: anyWindow, check: checkCNRIC, need: needNothing,
			words: []string{"身份证", "resident identity", "id card"}, distinct: true,
		},
		{
			typ:     entity.CreditCard,
			lens:    span(13, 19),
			windows: digitWindow,
			check:   checkCreditCard,
			need:    needNothing,
			words:   cardWords,
			layouts: cardLayouts,
		},
		{
			typ:     entity.IBAN,
			lens:    span(15, 34),
			windows: alnumWindow,
			check:   checkIBAN,
			need:    needNothing,
			loose:   true,
		},
	}
}

func hetuWords() []string {
	return []string{"hetu", "henkilötunnus", "henkilotunnus", "personal identity code"}
}

type ruleIndex struct {
	digits [maxCompact + 1][]*rule
	alnum  [maxCompact + 1][]*rule
	byType [entity.Count]*rule
}

func newRuleIndex() *ruleIndex {
	ix := &ruleIndex{}
	rs := ruleSet()
	for i := range rs {
		r := &rs[i]
		r.keys = toBytes(r.words)
		if ix.byType[r.typ] == nil {
			ix.byType[r.typ] = r
		}
		for _, n := range r.lens {
			if r.windows&digitWindow != 0 {
				ix.digits[n] = append(ix.digits[n], r)
			}
			if r.windows&alnumWindow != 0 {
				ix.alnum[n] = append(ix.alnum[n], r)
			}
		}
	}
	return ix
}

func (s *scanner) layout(r *rule, first, last int) bool {
	if first == last {
		return false
	}
	for _, l := range r.layouts {
		if s.sizes(first, last, l...) {
			return true
		}
	}
	return false
}

type classifier struct {
	s      *scanner
	b      []byte
	rs     []*rule
	first  int
	last   int
	single bool
	ctx    *lowered
	valid  [16]int8
}

func (c *classifier) ok(k int) bool {
	r := c.rs[k]
	if r.single && !c.single {
		return false
	}
	if k >= len(c.valid) {
		return r.check.valid(c.b)
	}
	if c.valid[k] == 0 {
		c.valid[k] = -1
		if r.check.valid(c.b) {
			c.valid[k] = 1
		}
	}
	return c.valid[k] == 1
}

func (c *classifier) byKeyword() entity.Type {
	for k, r := range c.rs {
		if r.keys != nil && c.ok(k) && c.s.lazy(c.ctx).has(true, r.keys) {
			return r.typ
		}
	}
	return entity.Unknown
}

func (c *classifier) byLayout() entity.Type {
	if c.single || !c.s.wholeRun(c.first, c.last) {
		return entity.Unknown
	}
	for k, r := range c.rs {
		if r.need >= needLayout && c.s.layout(r, c.first, c.last) && c.ok(k) {
			return r.typ
		}
	}
	return entity.Unknown
}

func (c *classifier) bare() entity.Type {
	for k, r := range c.rs {
		if r.need == needNothing && c.ok(k) {
			return r.typ
		}
	}
	return entity.Unknown
}

func (s *scanner) classify(b []byte, digitsOnly bool, first, last int, ctx *lowered) entity.Type {
	if len(b) > maxCompact {
		return entity.Unknown
	}
	rs := s.d.rules.alnum[len(b)]
	if digitsOnly {
		rs = s.d.rules.digits[len(b)]
	}
	if len(rs) == 0 {
		return entity.Unknown
	}
	c := classifier{s: s, b: b, rs: rs, first: first, last: last, single: first == last, ctx: ctx}
	if t := c.byKeyword(); t != entity.Unknown {
		return t
	}
	if t := c.byLayout(); t != entity.Unknown {
		return t
	}
	return c.bare()
}

func (s *scanner) wholeRun(first, last int) bool {
	return (first == 0 || !s.digitGroup(first-1)) && (last >= s.run-1 || !s.digitGroup(last+1))
}

func (s *scanner) idScore(t entity.Type, first, last int) float32 {
	r := s.d.rules.byType[t]
	if r == nil || r.keys == nil {
		return baseScore(t)
	}
	switch {
	case s.keyword(s.groups[first].start, r.keys):
		return 1
	case s.layout(r, first, last):
		return formattedScore
	case r.bare != 0:
		return r.bare
	case r.distinct:
		return formattedScore
	}
	return bareIDScore
}

func (s *scanner) looseGroups(t entity.Type, first, last int) bool {
	if first >= last || t == entity.IP || t == entity.IPPrivate {
		return false
	}
	r := s.d.rules.byType[t]
	return r != nil && !r.distinct && !r.loose && !s.layout(r, first, last)
}
