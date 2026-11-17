package entity

type Type uint8

const (
	Unknown Type = iota
	Email
	Phone
	IP
	IPPrivate
	CPF
	CNPJ
	PTNIF
	ESDNI
	ESNIE
	DESteuerID
	FRNIR
	ITCF
	IBAN
	CreditCard
	NLBSN
	PLPESEL
	BENN
	ATSVNR
	SEPNR
	FIHETU
	IEPPSN
	UKNINO
	BRPIS
	Person
	typeCount
)

const (
	unknownName = "UNKNOWN"
	nameEmail   = "EMAIL"
	namePhone   = "PHONE"
	nameCpf     = "CPF"
	nameCnpj    = "CNPJ"
	nameIban    = "IBAN"
)

const (
	MaxNameLen  = 12
	MaxLabelLen = 8
)

func (t Type) Label() string {
	switch t {
	case Email:
		return nameEmail
	case Phone:
		return namePhone
	case IP, IPPrivate:
		return "IP"
	case CPF:
		return nameCpf
	case CNPJ:
		return nameCnpj
	case PTNIF:
		return "NIF"
	case ESDNI:
		return "DNI"
	case ESNIE:
		return "NIE"
	case DESteuerID:
		return "STEUERID"
	case FRNIR:
		return "NIR"
	case ITCF:
		return "CF"
	case IBAN:
		return nameIban
	case CreditCard:
		return "CARD"
	case NLBSN:
		return "BSN"
	case PLPESEL:
		return "PESEL"
	case BENN:
		return "NN"
	case ATSVNR:
		return "SVNR"
	case SEPNR:
		return "PNR"
	case FIHETU:
		return "HETU"
	case IEPPSN:
		return "PPSN"
	case UKNINO:
		return "NINO"
	case BRPIS:
		return "PIS"
	case Person:
		return "NAME"
	case Unknown, typeCount:
		return unknownName
	}
	return unknownName
}

func (t Type) LabelLower() string {
	switch t {
	case Email:
		return "email"
	case Phone:
		return "phone"
	case IP, IPPrivate:
		return "ip"
	case CPF:
		return "cpf"
	case CNPJ:
		return "cnpj"
	case PTNIF:
		return "nif"
	case ESDNI:
		return "dni"
	case ESNIE:
		return "nie"
	case DESteuerID:
		return "steuerid"
	case FRNIR:
		return "nir"
	case ITCF:
		return "cf"
	case IBAN:
		return "iban"
	case CreditCard:
		return "card"
	case NLBSN:
		return "bsn"
	case PLPESEL:
		return "pesel"
	case BENN:
		return "nn"
	case ATSVNR:
		return "svnr"
	case SEPNR:
		return "pnr"
	case FIHETU:
		return "hetu"
	case IEPPSN:
		return "ppsn"
	case UKNINO:
		return "nino"
	case BRPIS:
		return "pis"
	case Person:
		return "name"
	case Unknown, typeCount:
		return "unknown"
	}
	return "unknown"
}

func LabelFold(b []byte) (Type, bool) {
	if len(b) == 0 || len(b) > MaxLabelLen {
		return Unknown, false
	}
	var up [MaxLabelLen]byte
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		up[i] = c
	}
	switch string(up[:len(b)]) {
	case nameEmail:
		return Email, true
	case namePhone:
		return Phone, true
	case "IP":
		return IP, true
	case nameCpf:
		return CPF, true
	case nameCnpj:
		return CNPJ, true
	case "NIF":
		return PTNIF, true
	case "DNI":
		return ESDNI, true
	case "NIE":
		return ESNIE, true
	case "STEUERID":
		return DESteuerID, true
	case "NIR":
		return FRNIR, true
	case "CF":
		return ITCF, true
	case nameIban:
		return IBAN, true
	case "CARD":
		return CreditCard, true
	case "BSN":
		return NLBSN, true
	case "PESEL":
		return PLPESEL, true
	case "NN":
		return BENN, true
	case "SVNR":
		return ATSVNR, true
	case "PNR":
		return SEPNR, true
	case "HETU":
		return FIHETU, true
	case "PPSN":
		return IEPPSN, true
	case "NINO":
		return UKNINO, true
	case "PIS":
		return BRPIS, true
	case "NAME":
		return Person, true
	}
	return Unknown, false
}

const Count = int(typeCount)

func (t Type) String() string {
	switch t {
	case Email:
		return nameEmail
	case Phone:
		return namePhone
	case IP:
		return "IP"
	case IPPrivate:
		return "IP_PRIVATE"
	case CPF:
		return nameCpf
	case CNPJ:
		return nameCnpj
	case PTNIF:
		return "PT_NIF"
	case ESDNI:
		return "ES_DNI"
	case ESNIE:
		return "ES_NIE"
	case DESteuerID:
		return "DE_STEUER_ID"
	case FRNIR:
		return "FR_NIR"
	case ITCF:
		return "IT_CF"
	case IBAN:
		return nameIban
	case CreditCard:
		return "CREDIT_CARD"
	case NLBSN:
		return "NL_BSN"
	case PLPESEL:
		return "PL_PESEL"
	case BENN:
		return "BE_NN"
	case ATSVNR:
		return "AT_SVNR"
	case SEPNR:
		return "SE_PNR"
	case FIHETU:
		return "FI_HETU"
	case IEPPSN:
		return "IE_PPSN"
	case UKNINO:
		return "UK_NINO"
	case BRPIS:
		return "BR_PIS"
	case Person:
		return "PERSON"
	case Unknown, typeCount:
		return unknownName
	}
	return unknownName
}

func (t Type) Valid() bool {
	return t > Unknown && t < typeCount
}

func All() []Type {
	out := make([]Type, 0, typeCount-1)
	for t := Unknown + 1; t < typeCount; t++ {
		out = append(out, t)
	}
	return out
}

func Parse(s string) (Type, bool) {
	for t := Unknown + 1; t < typeCount; t++ {
		if t.String() == s {
			return t, true
		}
	}
	return Unknown, false
}

func ParseFold(b []byte) (Type, bool) {
	if len(b) == 0 || len(b) > MaxNameLen {
		return Unknown, false
	}
	var up [MaxNameLen]byte
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		up[i] = c
	}
	for t := Unknown + 1; t < typeCount; t++ {
		if t.String() == string(up[:len(b)]) {
			return t, true
		}
	}
	return Unknown, false
}

type Span struct {
	Start int
	End   int
	Type  Type
	Score float32
}

func (s Span) Len() int {
	return s.End - s.Start
}

func (s Span) Overlaps(o Span) bool {
	return s.Start < o.End && o.Start < s.End
}

func Priority(t Type) int {
	switch t {
	case CPF, CNPJ, PTNIF, ESDNI, ESNIE, FRNIR, ITCF, IBAN, CreditCard, NLBSN, PLPESEL, BENN, ATSVNR, SEPNR, FIHETU,
		IEPPSN, UKNINO, BRPIS:
		return 4
	case Email:
		return 3
	case DESteuerID, IP, IPPrivate:
		return 2
	case Phone, Person:
		return 1
	case Unknown, typeCount:
		return 0
	}
	return 0
}

type Mask uint64

func (m Mask) Has(t Type) bool {
	return m&(1<<uint(t)) != 0
}

func (m Mask) With(t Type) Mask {
	return m | 1<<uint(t)
}

func AllMask() Mask {
	var m Mask
	for t := Unknown + 1; t < typeCount; t++ {
		m = m.With(t)
	}
	return m
}
