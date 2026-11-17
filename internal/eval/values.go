package eval

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers"

	"github.com/Guilherme-Ciano/maskwire/internal/detect/checksum"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

const maxTries = 1 << 20

type value struct {
	typ       entity.Type
	formatted string
	compact   string
	keyword   string
	needsKey  bool
}

type gen struct {
	r *rand.Rand
}

func (g *gen) digits(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = "0123456789"[g.r.IntN(10)]
	}
	return string(b)
}

func (g *gen) upper(n int, alphabet string) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[g.r.IntN(len(alphabet))]
	}
	return string(b)
}

func (g *gen) pick(xs ...string) string {
	return xs[g.r.IntN(len(xs))]
}

func (g *gen) until(mk func() string, valid func([]byte) bool) string {
	for range maxTries {
		if s := mk(); valid([]byte(s)) {
			return s
		}
	}
	panic("eval: generator never produced a valid value")
}

func group(s string, sizes ...int) string {
	var b strings.Builder
	for i, n := range sizes {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[:n])
		s = s[n:]
	}
	return b.String()
}

func (g *gen) date() (dd, mm, yy string) {
	return fmt.Sprintf("%02d", 1+g.r.IntN(28)), fmt.Sprintf("%02d", 1+g.r.IntN(12)), fmt.Sprintf("%02d", g.r.IntN(100))
}

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func (g *gen) cpf() value {
	c := g.until(func() string { return g.digits(11) }, checksum.CPF)
	return value{
		entity.CPF,
		c[:3] + "." + c[3:6] + "." + c[6:9] + "-" + c[9:],
		c,
		g.pick("CPF", "cpf do cliente:"),
		false,
	}
}

func (g *gen) cnpj() value {
	mk := func() string { return g.digits(14) }
	if g.r.IntN(3) == 0 {
		mk = func() string { return g.upper(12, "0123456789ABCDEFGHJKLMNPRSTUVWXYZ") + g.digits(2) }
	}
	c := g.until(mk, checksum.CNPJ)
	return value{entity.CNPJ, c[:2] + "." + c[2:5] + "." + c[5:8] + "/" + c[8:12] + "-" + c[12:], c, "CNPJ", false}
}

func (g *gen) pis() value {
	c := g.until(func() string { return g.digits(11) }, checksum.BRPIS)
	return value{entity.BRPIS, c[:3] + "." + c[3:8] + "." + c[8:10] + "-" + c[10:], c, "PIS", true}
}

func (g *gen) nif() value {
	c := g.until(func() string { return g.pick("1", "2", "3", "5", "6", "8", "9") + g.digits(8) }, checksum.PTNIF)
	return value{entity.PTNIF, c, c, g.pick("NIF", "contribuinte n.º"), false}
}

const dniLetters = "TRWAGMYFPDXBNJZSQVHLCKE"

func (g *gen) dni() value {
	v := g.r.IntN(100_000_000)
	c := fmt.Sprintf("%08d%c", v, dniLetters[v%23])
	return value{entity.ESDNI, c, c, "DNI", false}
}

func (g *gen) nie() value {
	c := g.until(func() string { return g.pick("X", "Y", "Z") + g.digits(7) + g.upper(1, dniLetters) }, checksum.ESNIE)
	return value{entity.ESNIE, c, c, "NIE", false}
}

func (g *gen) steuer() value {
	c := g.until(
		func() string { return g.pick("1", "2", "3", "4", "5", "6", "7", "8", "9") + g.digits(10) },
		checksum.DESteuerID,
	)
	return value{entity.DESteuerID, group(c, 2, 3, 3, 3), c, "Steuer-ID", false}
}

func (g *gen) nir() value {
	_, mm, yy := g.date()
	body := g.pick("1", "2") + yy + mm + fmt.Sprintf("%02d", 1+g.r.IntN(95)) + g.digits(3) + g.digits(3)
	var n uint64
	for i := range len(body) {
		n = n*10 + uint64(body[i]-'0')
	}
	c := body + fmt.Sprintf("%02d", 97-n%97)
	return value{entity.FRNIR, group(c, 1, 2, 2, 2, 3, 3, 2), c, "NIR", false}
}

func (g *gen) cf() value {
	_, _, yy := g.date()
	day := 1 + g.r.IntN(28)
	if g.r.IntN(2) == 0 {
		day += 40
	}
	dd := fmt.Sprintf("%02d", day)
	base := g.upper(6, letters) + yy + g.upper(1, "ABCDEHLMPRST") + dd + g.upper(1, letters) + g.digits(3)
	c := g.until(func() string { return base + g.upper(1, letters) }, checksum.ITCF)
	return value{entity.ITCF, c, c, "codice fiscale", false}
}

func ibanCheck(country, bban string) string {
	var s strings.Builder
	for _, ch := range bban + country + "00" {
		if ch >= 'A' && ch <= 'Z' {
			s.WriteString(strconv.Itoa(int(ch-'A') + 10))
		} else {
			s.WriteRune(ch)
		}
	}
	r := 0
	for _, ch := range s.String() {
		r = (r*10 + int(ch-'0')) % 97
	}
	return fmt.Sprintf("%02d", 98-r)
}

func (g *gen) iban() value {
	country, n := []string{"DE", "PT", "ES", "FR", "IT", "NL", "BE", "AT"}[g.r.IntN(8)], 0
	var bban string
	switch country {
	case "DE":
		n = 18
	case "PT":
		n = 21
	case "ES":
		n = 20
	case "FR":
		n = 23
	case "BE":
		n = 12
	case "AT":
		n = 16
	}
	switch country {
	case "IT":
		bban = g.upper(1, letters) + g.digits(22)
	case "NL":
		bban = g.upper(4, letters) + g.digits(10)
	default:
		bban = g.digits(n)
	}
	c := country + ibanCheck(country, bban) + bban
	var f strings.Builder
	for i := 0; i < len(c); i += 4 {
		if i > 0 {
			f.WriteByte(' ')
		}
		f.WriteString(c[i:min(i+4, len(c))])
	}
	return value{entity.IBAN, f.String(), c, "IBAN", false}
}

func (g *gen) card() value {
	c := g.until(func() string {
		p := g.pick("4", "51", "52", "53", "54", "55")
		return p + g.digits(16-len(p))
	}, checksum.CreditCard)
	return value{entity.CreditCard, group(c, 4, 4, 4, 4), c, g.pick("card", "cartão", "tarjeta"), false}
}

func (g *gen) bsn() value {
	c := g.until(func() string { return g.digits(9) }, checksum.NLBSN)
	return value{entity.NLBSN, c, c, "BSN", true}
}

func (g *gen) pesel() value {
	_, mm, yy := g.date()
	c := g.until(func() string { return yy + mm + fmt.Sprintf("%02d", 1+g.r.IntN(28)) + g.digits(5) }, checksum.PLPESEL)
	return value{entity.PLPESEL, c, c, "PESEL", true}
}

func (g *gen) benn() value {
	dd, mm, yy := g.date()
	c := g.until(func() string { return yy + mm + dd + g.digits(5) }, checksum.BENN)
	return value{
		entity.BENN,
		c[:2] + "." + c[2:4] + "." + c[4:6] + "-" + c[6:9] + "." + c[9:],
		c,
		"rijksregister",
		true,
	}
}

func (g *gen) svnr() value {
	dd, mm, yy := g.date()
	c := g.until(
		func() string { return g.pick("1", "2", "3", "4", "5", "6", "7", "8", "9") + g.digits(3) + dd + mm + yy },
		checksum.ATSVNR,
	)
	return value{entity.ATSVNR, c[:4] + " " + c[4:], c, "SVNR", true}
}

func (g *gen) pnr() value {
	dd, mm, yy := g.date()
	c := g.until(func() string { return yy + mm + dd + g.digits(4) }, checksum.SEPNR)
	return value{entity.SEPNR, c[:6] + "-" + c[6:], c, "personnummer", false}
}

func (g *gen) hetu() value {
	dd, mm, yy := g.date()
	sep := g.pick("-", "A")
	c := g.until(
		func() string { return dd + mm + yy + g.digits(3) + g.upper(1, "0123456789ABCDEFHJKLMNPRSTUVWXY") },
		checksum.FIHETU,
	)
	return value{entity.FIHETU, c[:6] + sep + c[6:], c, "henkilötunnus", false}
}

func (g *gen) ppsn() value {
	c := g.until(func() string { return g.digits(7) + g.upper(1, letters) }, checksum.IEPPSN)
	return value{entity.IEPPSN, c, c, "PPSN", false}
}

func (g *gen) nino() value {
	c := g.until(func() string { return g.upper(2, letters) + g.digits(6) + g.upper(1, "ABCD") }, checksum.UKNINO)
	return value{entity.UKNINO, group(c, 2, 2, 2, 2, 1), c, "NI number", false}
}

func firstNames() []string {
	return []string{"ana", "joao", "maria", "pedro", "lucia", "hans", "claire", "giulia", "pablo", "sofia"}
}

func (g *gen) email() value {
	user := g.pick(
		firstNames()...) + g.pick(
		".",
		"_",
		"",
	) + g.pick(
		"silva",
		"costa",
		"muller",
		"rossi",
		"garcia",
		"martin",
	) +
		g.pick(
			"",
			strconv.Itoa(g.r.IntN(100)),
		)
	domain := g.pick(
		"empresa",
		"correio",
		"acme",
		"firma",
		"societe",
		"azienda",
	) + g.pick(
		".com.br",
		".pt",
		".de",
		".fr",
		".it",
		".es",
		".com",
	)
	c := user + "@" + domain
	f := c
	if g.r.IntN(10) == 0 {
		f = user + " (at) " + strings.Replace(domain, ".", " (dot) ", 1)
	}
	return value{entity.Email, f, c, "e-mail", false}
}

type phoneShape struct {
	region, prefix string
	national       func(g *gen) string
}

func phoneShapes() []phoneShape {
	return []phoneShape{
		{
			"BR",
			"+55",
			func(g *gen) string { return g.pick("11", "21", "31", "41") + " 9" + g.digits(4) + "-" + g.digits(4) },
		},
		{"PT", "+351", func(g *gen) string {
			return "9" + g.pick("1", "2", "3", "6") + g.digits(1) + " " + g.digits(3) + " " + g.digits(3)
		}},
		{"ES", "+34", func(g *gen) string { return "6" + g.digits(2) + " " + g.digits(3) + " " + g.digits(3) }},
		{
			"DE",
			"+49",
			func(g *gen) string { return "15" + g.pick("1", "2", "7", "9") + g.digits(1) + " " + g.digits(7) },
		},
		{"FR", "+33", func(g *gen) string {
			return g.pick("6", "7") + " " + g.digits(2) + " " + g.digits(2) + " " + g.digits(2) + " " + g.digits(2)
		}},
		{"IT", "+39", func(g *gen) string { return "3" + g.digits(2) + " " + g.digits(3) + " " + g.digits(4) }},
	}
}

func (g *gen) phone() value {
	shapes := phoneShapes()
	for range maxTries {
		s := shapes[g.r.IntN(len(shapes))]
		nat := s.national(g)
		num, err := phonenumbers.Parse(s.prefix+" "+nat, s.region)
		if err != nil || !phonenumbers.IsValidNumber(num) {
			continue
		}
		return value{entity.Phone, s.prefix + " " + nat, nat, g.pick("tel", "telefone", "Telefon", "teléfono"), false}
	}
	panic("eval: no valid phone")
}

func (g *gen) ip() value {
	if g.r.IntN(4) == 0 {
		c := fmt.Sprintf("2a0%d:%x:%x::%x", 1+g.r.IntN(9), g.r.IntN(65536), g.r.IntN(65536), 1+g.r.IntN(65535))
		return value{entity.IP, c, c, "IP", false}
	}
	for {
		a := 1 + g.r.IntN(223)
		if a == 10 || a == 127 || a == 100 || a == 169 || a == 172 || a == 192 {
			continue
		}
		c := fmt.Sprintf("%d.%d.%d.%d", a, g.r.IntN(256), g.r.IntN(256), 1+g.r.IntN(254))
		return value{entity.IP, c, c, "IP", false}
	}
}

func surnames() []string {
	return []string{
		"Silva", "Souza", "Santos", "Oliveira", "Pereira", "Costa", "Rodrigues", "Almeida", "Nascimento", "Lima",
		"Araújo", "Fernandes", "Carvalho", "Gomes", "Martins", "Rocha", "Ribeiro", "Alves", "Monteiro", "Mendes",
		"García", "Martínez", "López", "Sánchez", "Pérez", "Gómez", "Fernández", "Ruiz", "Moreno", "Jiménez",
		"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker", "Hoffmann", "Koch",
		"Dubois", "Bernard", "Moreau", "Laurent", "Lefebvre", "Girard", "Rossi", "Russo", "Ferrari", "Esposito",
		"Bianchi", "Romano", "Colombo", "Ricci", "Marino", "Greco", "Bruno", "Gallo", "Conti", "De Luca",
		"Vasconcelos", "Magalhães", "Albuquerque", "Figueiredo", "Bittencourt", "Kowalski", "Nowak", "Jansen",
	}
}

func (g *gen) person() value {
	first := g.pick("Ana", "João", "Maria", "Pedro", "Lucas", "Juliana", "Carlos", "Fernanda", "Hans", "Claire",
		"Giulia", "Pablo", "Sofía", "Matteo", "Camille", "Thomas", "Beatriz", "Rafael", "Laura", "Diego")
	name := first + " " + g.pick(surnames()...)
	if g.r.IntN(3) == 0 {
		name += " " + g.pick(surnames()...)
	}
	return value{
		entity.Person,
		name,
		strings.ToLower(name),
		g.pick("Sr.", "Sra.", "cliente:", "nome:", "Frau", "Mme"),
		false,
	}
}

func (g *gen) values() []func() value {
	return []func() value{
		g.cpf, g.cnpj, g.pis, g.nif, g.dni, g.nie, g.steuer, g.nir, g.cf, g.iban, g.card, g.bsn, g.pesel, g.benn,
		g.svnr, g.pnr, g.hetu, g.ppsn, g.nino, g.email, g.phone, g.ip, g.person,
	}
}
