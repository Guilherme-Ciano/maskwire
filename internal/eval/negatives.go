package eval

import (
	"fmt"
	"strconv"

	"github.com/Guilherme-Ciano/maskwire/internal/detect/checksum"
)

type negative struct {
	kind string
	text string
}

func (g *gen) nearMiss(v string, valid func([]byte) bool) string {
	b := []byte(v)
	for range maxTries {
		i := g.r.IntN(len(b))
		if b[i] < '0' || b[i] > '9' {
			continue
		}
		old := b[i]
		b[i] = byte('0' + (int(old-'0')+1+g.r.IntN(9))%10)
		if !valid(b) {
			return string(b)
		}
		b[i] = old
	}
	panic("eval: no near miss")
}

func (g *gen) negatives() []func() negative {
	return []func() negative{
		func() negative {
			return negative{
				"order number (5-8 digits)",
				g.pick("pedido", "order", "Bestellung", "commande") + " " + g.digits(5+g.r.IntN(4)),
			}
		},
		func() negative {
			c := g.nearMiss(g.cpf().compact, checksum.CPF)
			return negative{"CPF with a wrong check digit", c[:3] + "." + c[3:6] + "." + c[6:9] + "-" + c[9:]}
		},
		func() negative {
			return negative{"IBAN with a wrong check digit", "IBAN " + g.nearMiss(g.iban().compact, checksum.IBAN)}
		},
		func() negative {
			c := g.nearMiss(g.card().compact, checksum.CreditCard)
			return negative{"card number failing Luhn", c[:4] + " " + c[4:8] + " " + c[8:12] + " " + c[12:]}
		},
		func() negative {
			dd, mm, _ := g.date()
			return negative{
				"date",
				g.pick(dd+"/"+mm+"/20"+g.digits(2), "20"+g.digits(2)+"-"+mm+"-"+dd, dd+"."+mm+".19"+g.digits(2)),
			}
		},
		func() negative {
			return negative{"time", fmt.Sprintf("%02d:%02d:%02d", g.r.IntN(24), g.r.IntN(60), g.r.IntN(60))}
		},
		func() negative {
			return negative{
				"price",
				g.pick("R$ ", "€ ", "EUR ") + strconv.Itoa(1+g.r.IntN(99)) + "." + g.digits(3) + "," + g.digits(2),
			}
		},
		func() negative {
			h := func(n int) string { return g.upper(n, "0123456789abcdef") }
			return negative{"UUID", h(8) + "-" + h(4) + "-4" + h(3) + "-a" + h(3) + "-" + h(12)}
		},
		func() negative {
			return negative{"git commit hash", "commit " + g.upper(40, "0123456789abcdef")}
		},
		func() negative {
			return negative{
				"version string",
				g.pick("v", "version ") + fmt.Sprintf("%d.%d.%d", g.r.IntN(20), g.r.IntN(50), g.r.IntN(300)),
			}
		},
		func() negative {
			return negative{
				"build number (4 dotted parts)",
				fmt.Sprintf("build %d.0.%d.%d", 10+g.r.IntN(5), 1000+g.r.IntN(30000), 256+g.r.IntN(5000)),
			}
		},
		func() negative {
			return negative{"unix timestamp", "ts " + strconv.Itoa(1_600_000_000+g.r.IntN(200_000_000))}
		},
		func() negative {
			return negative{
				"random 9 digits after a non-ID word",
				g.pick("ticket", "protocolo", "invoice", "Rechnung") + " " + g.digits(9),
			}
		},
		func() negative {
			return negative{
				"random 11 digits after a non-ID word",
				g.pick("protocolo", "tracking", "Vorgang", "dossier") + " " + g.digits(11),
			}
		},
		func() negative {
			return negative{
				"reserved-domain e-mail",
				g.pick(firstNames()...) + "@example." + g.pick("com", "org", "net"),
			}
		},
		func() negative {
			return negative{
				"place or institution containing a first name",
				g.pick(
					"São Paulo",
					"Santa Catarina",
					"San Francisco",
					"Vila Isabel",
					"Porto Alegre",
					"Rua Augusta",
					"Avenida Paulista",
					"Hospital Albert Einstein",
					"Saint Louis",
					"Santa Maria",
					"Rio de Janeiro",
					"Banco Santander",
					"Jardim Paulista",
				),
			}
		},
		func() negative {
			return negative{"capitalized common words", g.pick("Obrigado pela Atenção", "Segue o Relatório",
				"Prezado Cliente", "Bom Dia", "Thank You", "Best Regards", "Atenciosamente Equipe", "Nota Fiscal")}
		},
		func() negative {
			return negative{
				"prose after a credential word",
				g.pick("password: please reset it", "token: expired yesterday",
					"secret: not shared", "api_key: see the vault"),
			}
		},
		func() negative {
			return negative{"quantity with unit", strconv.Itoa(1+g.r.IntN(9999)) + g.pick(" kg", " km", " ml", " MB")}
		},
	}
}
