package detect

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

//go:embed firstnames.txt
var firstNamesList string

const (
	maxNameWords      = 5
	maxWordBytes      = 32
	nameScore         = 0.85
	contextNameScore  = 0.9
	corefNameScore    = 0.8
	lonelyFirstScore  = 0.5
	minCorefPartBytes = 3
)

type wordClass uint8

const (
	classParticle wordClass = 1 << iota
	classHonorific
	classLabel
	classBlocker
	classNotSurname
	classFirstName
)

type gazetteer struct {
	words map[string]wordClass
	pool  sync.Pool
}

func addWords(m map[string]wordClass, c wordClass, words string) {
	for w := range strings.FieldsSeq(words) {
		m[w] |= c
	}
}

func newGazetteer() *gazetteer {
	m := map[string]wordClass{}
	addWords(m, classFirstName, firstNamesList)
	addWords(m, classParticle, "da de do das dos di del della van von der den du la le y e zu ten ter dal degli")
	addWords(m, classHonorific, "sr sra srta dr dra dona senhor senhora don doña señor señora herr frau mme mlle "+
		"monsieur madame sig sigra signor signora dott dottssa mr mrs ms miss prof profa eng")
	addWords(m, classLabel, "nome name nombre nom cliente customer client paciente patient titular beneficiário "+
		"beneficiario contato contacto attn kunde kundin destinatário destinatario")
	addWords(m, classBlocker, "são sao san santa santo saint st ste rua avenida av calle via rue praça praca "+
		"travessa rodovia hospital escola colégio colegio universidade universidad université banco igreja "+
		"instituto fundação fundacao edifício edificio condomínio condominio shopping estação estacao aeroporto "+
		"porto vila jardim parque")
	addWords(m, classNotSurname, "janeiro fevereiro março abril maio junho julho agosto setembro outubro novembro "+
		"dezembro january february march april june july august september october november december enero "+
		"febrero marzo mayo junio julio septiembre octubre noviembre diciembre segunda terça quarta quinta sexta "+
		"sábado domingo monday tuesday wednesday thursday friday saturday sunday ltda inc ltd gmbh street road "+
		"avenue the and obrigado obrigada olá ola hello hi dear caro cara prezado prezada estimado estimada bom "+
		"boa dia tarde noite please thanks thank merci danke bitte grazie gracias regards atenciosamente "+
		"cumprimentos saludos cordialement grüße grusse abraço abraco")
	return &gazetteer{words: m}
}

type nword struct {
	start, end int
	class      wordClass
	capital    bool
	covered    bool
	strong     bool
}

func (g *gazetteer) classify(buf, word []byte) wordClass {
	if len(word) > maxWordBytes {
		return 0
	}
	n := 0
	for i := 0; i < len(word); {
		c := word[i]
		if c < utf8.RuneSelf {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			buf[n] = c
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRune(word[i:])
		n += utf8.EncodeRune(buf[n:], unicode.ToLower(r))
		i += size
	}
	c := g.words[string(buf[:n])]
	if c&classFirstName == 0 {
		if dash := indexByte(buf[:n], '-'); dash > 0 {
			c |= g.words[string(buf[:dash])] & classFirstName
		}
	}
	return c
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func letterAt(t []byte, i int) (r rune, n int, ok bool) {
	if c := t[i]; c < utf8.RuneSelf {
		return rune(c), 1, (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	r, n = utf8.DecodeRune(t[i:])
	return r, n, unicode.IsLetter(r)
}

func isUpperRune(r rune) bool {
	if r < utf8.RuneSelf {
		return r >= 'A' && r <= 'Z'
	}
	return unicode.IsUpper(r)
}

func skipLower(t []byte, i int) int {
	for i < len(t) && (t[i] >= 'a' && t[i] <= 'z' || t[i] >= 'A' && t[i] <= 'Z' || t[i] >= utf8.RuneSelf) {
		i++
	}
	return i
}

func scanWord(t []byte, i int) (end int, capital bool) {
	r, n, _ := letterAt(t, i)
	upperRest, lowerSeen, afterJoin := false, false, false
	capital = isUpperRune(r)
	i += n
	for i < len(t) {
		r, n, ok := letterAt(t, i)
		if ok {
			switch {
			case !isUpperRune(r):
				lowerSeen = true
			case !afterJoin:
				upperRest = true
			}
			afterJoin = false
			i += n
			continue
		}
		if (r != '-' && r != '\'') || i+n >= len(t) {
			break
		}
		if _, _, next := letterAt(t, i+n); !next {
			break
		}
		afterJoin = true
		i += n
	}
	return i, capital && lowerSeen && !upperRest
}

func (s *scanner) nameWords(dst []nword) []nword {
	var buf [4 * maxWordBytes]byte
	t := s.text
	g := s.d.names
	for i := 0; i < len(t); {
		if c := t[i]; c >= 'a' && c <= 'z' {
			start := i
			i = skipLower(t, i)
			dst = append(dst, nword{start: start, end: i})
			continue
		}
		_, n, ok := letterAt(t, i)
		if !ok {
			i += n
			continue
		}
		start := i
		end, capital := scanWord(t, i)
		i = end
		if !capital && end-start > len("beneficiário") {
			continue
		}
		dst = append(dst, nword{start: start, end: end, capital: capital})
	}
	for k := range dst {
		if dst[k].capital || (k+1 < len(dst) && dst[k+1].capital) {
			dst[k].class = g.classify(buf[:], t[dst[k].start:dst[k].end])
		}
	}
	return dst
}

func (s *scanner) spaced(a, b nword, dot bool) bool {
	gap := s.text[a.end:b.start]
	if len(gap) == 0 || len(gap) > 3 {
		return false
	}
	for _, c := range gap {
		switch {
		case c == ' ', c == 0xC2, c == 0xA0:
		case (c == ':' || c == '.') && dot:
		default:
			return false
		}
	}
	return true
}

func surnameWord(w nword) bool {
	return w.capital && w.class&classNotSurname == 0
}

func (s *scanner) labelWord(w nword) bool {
	if w.class&classLabel == 0 {
		return false
	}
	return !w.capital || (w.end < len(s.text) && s.text[w.end] == ':')
}

func (s *scanner) nameRun(ws []nword, i int) int {
	end, words := i, 1
	for j := i + 1; j < len(ws) && words < maxNameWords; j++ {
		if !s.spaced(ws[j-1], ws[j], false) {
			break
		}
		if ws[j].class&classParticle != 0 && !ws[j].capital && j+1 < len(ws) && s.spaced(ws[j], ws[j+1], false) &&
			surnameWord(ws[j+1]) {
			continue
		}
		if !surnameWord(ws[j]) {
			break
		}
		end, words = j, words+1
	}
	return end
}

func (s *scanner) startsName(ws []nword, i int) bool {
	w := ws[i]
	if !w.capital || w.class&classFirstName == 0 {
		return false
	}
	return i == 0 || ws[i-1].class&classBlocker == 0 || !s.spaced(ws[i-1], w, true)
}

func (s *scanner) names() {
	g := s.d.names
	p, ok := g.pool.Get().(*[]nword)
	if !ok {
		p = new([]nword)
	}
	ws := s.nameWords((*p)[:0])
	strong := false
	for i := 0; i < len(ws); i++ {
		w := ws[i]
		switch {
		case (w.class&classHonorific != 0 || s.labelWord(w)) && i+1 < len(ws) && s.spaced(w, ws[i+1], true) &&
			surnameWord(ws[i+1]):
			end := s.nameRun(ws, i+1)
			s.emitName(ws, i+1, end, contextNameScore)
			strong = true
			i = end
		case s.startsName(ws, i):
			end := s.nameRun(ws, i)
			score := float32(nameScore)
			if end == i {
				score = lonelyFirstScore
			} else {
				strong = true
			}
			s.emitName(ws, i, end, score)
			i = end
		}
	}
	if strong {
		s.coref(ws)
	}
	*p = ws[:0]
	g.pool.Put(p)
}

func (s *scanner) coref(ws []nword) {
	parts := map[string]bool{}
	for _, w := range ws {
		if w.strong && w.capital && w.end-w.start >= minCorefPartBytes {
			parts[string(s.text[w.start:w.end])] = true
		}
	}
	for _, w := range ws {
		if !w.covered && w.capital && parts[string(s.text[w.start:w.end])] {
			s.emitScore(w.start, w.end, entity.Person, corefNameScore)
		}
	}
}

func (s *scanner) emitName(ws []nword, first, last int, score float32) {
	s.emitScore(ws[first].start, ws[last].end, entity.Person, score)
	for k := first; k <= last; k++ {
		ws[k].covered = true
		ws[k].strong = score >= nameScore
	}
}
