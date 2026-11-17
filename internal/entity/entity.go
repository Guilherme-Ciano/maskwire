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
	USSSN
	USITIN
	USEIN
	USABA
	MXCURP
	MXRFC
	ARCUIT
	CLRUT
	CONIT
	PERUC
	UYCI
	INAadhaar
	INPAN
	AUTFN
	AUABN
	NZIRD
	JPMyNumber
	KRRRN
	CNRIC
	SGNRIC
	ZAID
	ILID
	TRTCKN
	Secret
	DateOfBirth
	typeCount
)

const (
	MaxNameLen  = 12
	MaxLabelLen = 8
)

func (t Type) spec() spec {
	if t >= typeCount {
		return specs[Unknown]
	}
	return specs[t]
}

func (t Type) Label() string {
	return t.spec().label
}

func (t Type) LabelLower() string {
	return t.spec().lower
}

func (t Type) Canon() Canon {
	return t.spec().canon
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
	t, ok := labels[string(up[:len(b)])]
	return t, ok
}

const Count = int(typeCount)

func (t Type) String() string {
	return t.spec().name
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
	return int(t.spec().priority)
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
