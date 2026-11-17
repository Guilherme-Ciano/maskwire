package eval

import (
	"github.com/Guilherme-Ciano/maskwire/internal/detect/checksum"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func (g *gen) digitsValid(n int, prefix string, valid func([]byte) bool) string {
	return g.until(func() string { return prefix + g.digits(n-len(prefix)) }, valid)
}

func (g *gen) ssn() value {
	c := g.digitsValid(9, "", checksum.USSSN)
	return value{entity.USSSN, c[:3] + "-" + c[3:5] + "-" + c[5:], c, g.pick("SSN", "social security number"), false}
}

func (g *gen) itin() value {
	c := g.digitsValid(9, "9", checksum.USITIN)
	return value{entity.USITIN, c[:3] + "-" + c[3:5] + "-" + c[5:], c, "ITIN", false}
}

func (g *gen) ein() value {
	c := g.digitsValid(9, "", checksum.USEIN)
	return value{entity.USEIN, c[:2] + "-" + c[2:], c, "EIN", true}
}

func (g *gen) aba() value {
	c := g.digitsValid(9, "0", checksum.USABA)
	return value{entity.USABA, c, c, g.pick("routing number", "ABA"), true}
}

func (g *gen) curp() value {
	_, _, yy := g.date()
	dd, mm, _ := g.date()
	base := g.upper(4, letters) + yy + mm + dd + g.pick("H", "M") + g.upper(5, letters) + g.digits(1)
	c := g.until(func() string { return base + g.digits(1) }, checksum.MXCURP)
	return value{entity.MXCURP, c, c, "CURP", false}
}

func (g *gen) rfc() value {
	dd, mm, yy := g.date()
	c := g.until(func() string {
		return g.upper(4, letters) + yy + mm + dd + g.upper(2, letters+"0123456789") + g.upper(1, "0123456789A")
	}, checksum.MXRFC)
	return value{entity.MXRFC, c, c, "RFC", false}
}

func (g *gen) cuit() value {
	c := g.digitsValid(11, g.pick("20", "23", "27", "30"), checksum.ARCUIT)
	return value{entity.ARCUIT, c[:2] + "-" + c[2:10] + "-" + c[10:], c, g.pick("CUIT", "CUIL"), false}
}

func (g *gen) rut() value {
	c := g.until(func() string { return g.pick("1", "2") + g.digits(7) + g.upper(1, "0123456789K") }, checksum.CLRUT)
	return value{entity.CLRUT, c[:2] + "." + c[2:5] + "." + c[5:8] + "-" + c[8:], c, "RUT", false}
}

func (g *gen) nit() value {
	c := g.digitsValid(10, g.pick("8", "9"), checksum.CONIT)
	return value{entity.CONIT, c[:3] + "." + c[3:6] + "." + c[6:9] + "-" + c[9:], c, "NIT", false}
}

func (g *gen) ruc() value {
	c := g.digitsValid(11, g.pick("10", "20"), checksum.PERUC)
	return value{entity.PERUC, c, c, "RUC", true}
}

func (g *gen) uyci() value {
	c := g.digitsValid(8, g.pick("1", "2", "3", "4"), checksum.UYCI)
	return value{entity.UYCI, c[:1] + "." + c[1:4] + "." + c[4:7] + "-" + c[7:], c, "cédula", false}
}

func (g *gen) aadhaar() value {
	c := g.digitsValid(12, g.pick("2", "4", "6", "8"), checksum.INAadhaar)
	return value{entity.INAadhaar, group(c, 4, 4, 4), c, "Aadhaar", false}
}

func (g *gen) pan() value {
	c := g.upper(3, letters) + g.pick("P", "C", "H", "F") + g.upper(1, letters) + g.digits(4) + g.upper(1, letters)
	return value{entity.INPAN, c, c, "PAN", false}
}

func (g *gen) tfn() value {
	c := g.digitsValid(9, "", checksum.AUTFN)
	return value{entity.AUTFN, group(c, 3, 3, 3), c, g.pick("TFN", "tax file number"), true}
}

func (g *gen) abn() value {
	c := g.digitsValid(11, g.pick("1", "3", "5", "7"), checksum.AUABN)
	return value{entity.AUABN, group(c, 2, 3, 3, 3), c, "ABN", false}
}

func (g *gen) ird() value {
	c := g.digitsValid(9, "0", checksum.NZIRD)
	return value{entity.NZIRD, c[:3] + "-" + c[3:6] + "-" + c[6:], c, "IRD", true}
}

func (g *gen) mynumber() value {
	c := g.digitsValid(12, "", checksum.JPMyNumber)
	return value{entity.JPMyNumber, group(c, 4, 4, 4), c, g.pick("マイナンバー", "My Number"), true}
}

func (g *gen) rrn() value {
	dd, mm, yy := g.date()
	c := g.until(func() string { return yy + mm + dd + g.pick("1", "2", "3", "4") + g.digits(6) }, checksum.KRRRN)
	return value{entity.KRRRN, c[:6] + "-" + c[6:], c, "주민등록번호", false}
}

func (g *gen) cnric() value {
	dd, mm, yy := g.date()
	base := "11" + g.digits(4) + "19" + yy + mm + dd + g.digits(3)
	c := g.until(func() string { return base + g.upper(1, "0123456789X") }, checksum.CNRIC)
	return value{entity.CNRIC, c, c, "身份证", false}
}

func (g *gen) nric() value {
	c := g.until(
		func() string { return g.pick("S", "T", "F", "G") + g.digits(7) + g.upper(1, letters) },
		checksum.SGNRIC,
	)
	return value{entity.SGNRIC, c, c, "NRIC", false}
}

func (g *gen) zaid() value {
	dd, mm, yy := g.date()
	c := g.until(func() string { return yy + mm + dd + g.digits(4) + g.pick("0", "1") + g.digits(2) }, checksum.ZAID)
	return value{entity.ZAID, c, c, "ID number", true}
}

func (g *gen) ilid() value {
	c := g.digitsValid(9, "", checksum.ILID)
	return value{entity.ILID, c, c, "teudat zehut", true}
}

func (g *gen) tckn() value {
	c := g.digitsValid(11, g.pick("1", "2", "3", "4", "5"), checksum.TRTCKN)
	return value{entity.TRTCKN, c, c, "TC Kimlik No", true}
}

func (g *gen) secret() value {
	alnum := letters + "abcdefghijklmnopqrstuvwxyz0123456789"
	c := g.pick(
		"AKIA"+g.upper(16, letters+"0123456789"),
		"ghp_"+g.upper(36, alnum),
		"sk_live_"+g.upper(24, alnum),
		"xoxb-"+g.digits(12)+"-"+g.upper(24, alnum),
		"eyJ"+g.upper(20, alnum)+".eyJ"+g.upper(30, alnum)+"."+g.upper(43, alnum),
	)
	return value{entity.Secret, c, c, g.pick("api key", "token", "credentials"), false}
}

func (g *gen) dob() value {
	dd, mm, _ := g.date()
	yyyy := g.pick("19", "20") + g.digits(2)
	return value{
		entity.DateOfBirth, dd + "/" + mm + "/" + yyyy, dd + mm + yyyy,
		g.pick("data de nascimento:", "DOB", "born", "geboren am", "fecha de nacimiento"), true,
	}
}

func (g *gen) worldValues() []func() value {
	return []func() value{
		g.secret, g.dob,
		g.ssn, g.itin, g.ein, g.aba, g.curp, g.rfc, g.cuit, g.rut, g.nit, g.ruc, g.uyci, g.aadhaar, g.pan, g.tfn,
		g.abn, g.ird, g.mynumber, g.rrn, g.cnric, g.nric, g.zaid, g.ilid, g.tckn,
	}
}
