package entity

type Canon uint8

const (
	CanonRaw Canon = iota
	CanonLower
	CanonDigits
	CanonUpperAlnum
	CanonPerson
)

const (
	priorityLow = 1 + iota
	priorityMedium
	priorityHigh
	priorityID
)

type spec struct {
	name     string
	label    string
	lower    string
	priority int8
	canon    Canon
}

func id(name, label, lower string, canon Canon) spec {
	return spec{name: name, label: label, lower: lower, priority: priorityID, canon: canon}
}

var specs = [typeCount]spec{
	Unknown:   {name: "UNKNOWN", label: "UNKNOWN", lower: "unknown"},
	Email:     {name: "EMAIL", label: "EMAIL", lower: "email", priority: priorityHigh, canon: CanonLower},
	Phone:     {name: "PHONE", label: "PHONE", lower: "phone", priority: priorityLow, canon: CanonDigits},
	IP:        {name: "IP", label: "IP", lower: "ip", priority: priorityMedium, canon: CanonLower},
	IPPrivate: {name: "IP_PRIVATE", label: "IP", lower: "ip", priority: priorityMedium, canon: CanonLower},
	CPF:       id("CPF", "CPF", "cpf", CanonDigits),
	CNPJ:      id("CNPJ", "CNPJ", "cnpj", CanonUpperAlnum),
	PTNIF:     id("PT_NIF", "NIF", "nif", CanonDigits),
	ESDNI:     id("ES_DNI", "DNI", "dni", CanonUpperAlnum),
	ESNIE:     id("ES_NIE", "NIE", "nie", CanonUpperAlnum),
	DESteuerID: {
		name:     "DE_STEUER_ID",
		label:    "STEUERID",
		lower:    "steuerid",
		priority: priorityMedium,
		canon:    CanonDigits,
	},
	FRNIR:       id("FR_NIR", "NIR", "nir", CanonUpperAlnum),
	ITCF:        id("IT_CF", "CF", "cf", CanonUpperAlnum),
	IBAN:        id("IBAN", "IBAN", "iban", CanonUpperAlnum),
	CreditCard:  id("CREDIT_CARD", "CARD", "card", CanonDigits),
	NLBSN:       id("NL_BSN", "BSN", "bsn", CanonDigits),
	PLPESEL:     id("PL_PESEL", "PESEL", "pesel", CanonDigits),
	BENN:        id("BE_NN", "NN", "nn", CanonDigits),
	ATSVNR:      id("AT_SVNR", "SVNR", "svnr", CanonDigits),
	SEPNR:       id("SE_PNR", "PNR", "pnr", CanonDigits),
	FIHETU:      id("FI_HETU", "HETU", "hetu", CanonUpperAlnum),
	IEPPSN:      id("IE_PPSN", "PPSN", "ppsn", CanonUpperAlnum),
	UKNINO:      id("UK_NINO", "NINO", "nino", CanonUpperAlnum),
	BRPIS:       id("BR_PIS", "PIS", "pis", CanonDigits),
	Person:      {name: "PERSON", label: "NAME", lower: "name", priority: priorityLow, canon: CanonPerson},
	USSSN:       id("US_SSN", "SSN", "ssn", CanonDigits),
	USITIN:      id("US_ITIN", "ITIN", "itin", CanonDigits),
	USEIN:       id("US_EIN", "EIN", "ein", CanonDigits),
	USABA:       id("US_ABA", "ABA", "aba", CanonDigits),
	MXCURP:      id("MX_CURP", "CURP", "curp", CanonUpperAlnum),
	MXRFC:       id("MX_RFC", "RFC", "rfc", CanonUpperAlnum),
	ARCUIT:      id("AR_CUIT", "CUIT", "cuit", CanonDigits),
	CLRUT:       id("CL_RUT", "RUT", "rut", CanonUpperAlnum),
	CONIT:       id("CO_NIT", "NIT", "nit", CanonDigits),
	PERUC:       id("PE_RUC", "RUC", "ruc", CanonDigits),
	UYCI:        id("UY_CI", "CI", "ci", CanonDigits),
	INAadhaar:   id("IN_AADHAAR", "AADHAAR", "aadhaar", CanonDigits),
	INPAN:       id("IN_PAN", "PAN", "pan", CanonUpperAlnum),
	AUTFN:       id("AU_TFN", "TFN", "tfn", CanonDigits),
	AUABN:       id("AU_ABN", "ABN", "abn", CanonDigits),
	NZIRD:       id("NZ_IRD", "IRD", "ird", CanonDigits),
	JPMyNumber:  id("JP_MYNUMBER", "MYNUMBER", "mynumber", CanonDigits),
	KRRRN:       id("KR_RRN", "RRN", "rrn", CanonDigits),
	CNRIC:       id("CN_RIC", "RIC", "ric", CanonUpperAlnum),
	SGNRIC:      id("SG_NRIC", "NRIC", "nric", CanonUpperAlnum),
	ZAID:        id("ZA_ID", "ZAID", "zaid", CanonDigits),
	ILID:        id("IL_ID", "ILID", "ilid", CanonDigits),
	TRTCKN:      id("TR_TCKN", "TCKN", "tckn", CanonDigits),
	Secret:      id("SECRET", "SECRET", "secret", CanonRaw),
	DateOfBirth: {name: "DOB", label: "DOB", lower: "dob", priority: priorityMedium, canon: CanonDigits},
}

var labels = func() map[string]Type {
	m := make(map[string]Type, typeCount)
	for t := Unknown + 1; t < typeCount; t++ {
		if t != IPPrivate {
			m[specs[t].label] = t
		}
	}
	return m
}()
