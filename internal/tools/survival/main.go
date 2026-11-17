package main

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

var errUsage = errors.New("usage: survival gen|score [flags]")

type source struct {
	Task string
	Text string
}

type Prompt struct {
	ID     string `json:"id"`
	Hint   bool   `json:"hint"`
	Prompt string `json:"prompt"`
}

type Answer struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func sources() []source {
	return []source{
		{
			"Write a short, polite reply to this support ticket.",
			"Hello, I am Ana Souza (ana.souza@correio.pt, phone +351 912 345 678). My refund for order 7731 should go to IBAN PT50 0002 0123 1234 5678 9015 4. My CPF is 529.982.247-25.",
		},
		{
			"Translate this message into English.",
			"Olá, sou o Sr. Gustavo Moraes, CPF 111.444.777-35, e-mail gustavo.moraes@empresa.com.br. Por favor, atualizem meu cadastro com o telefone +55 11 98765-4321.",
		},
		{
			"Translate this message into Spanish.",
			"Please send the contract to claire.dubois@societe.fr and copy the finance team at finance@firma.de. Her NIR is 1 84 12 76 451 089 46.",
		},
		{
			"Summarise this ticket in two sentences, keeping every identifier.",
			"Customer Hans Müller (Steuer-ID 86095742719) reports a duplicate charge on card 4242 4242 4242 4242. He asks for a callback on +49 151 23456789 and an email to hans.mueller@post.de.",
		},
		{
			"Extract the fields name, email, phone and tax_id as a JSON object.",
			"Nueva alta: cliente Pablo García, DNI 12345678Z, correo pablo.garcia@correo.es, teléfono +34 612 345 678.",
		},
		{
			"Rewrite this message in a more formal tone.",
			"hey, it's giulia.rossi@azienda.it here, my codice fiscale is RSSMRA85T10A562S and my IBAN is IT60 X054 2811 1010 0000 0123 456. pls fix the invoice",
		},
		{
			"Write a bullet list of the actions the agent must take.",
			"Ticket 4410: Fernanda Lima (fernanda.lima@correio.com.br) wants her CNPJ 11.222.333/0001-81 added to invoices and her PIX key +55 21 99876-5432 updated.",
		},
		{
			"Write the arguments of a send_email tool call as JSON with fields to, subject and body.",
			"Tell bruno.costa@empresa.pt that the refund to IBAN PT50 0002 0123 1234 5678 9015 4 was approved and reference his NIF 123456789.",
		},
		{
			"Reply to the customer and confirm the data on file.",
			"Bonjour, je suis Mme Camille Bernard, mon numéro est +33 6 12 34 56 78 et mon e-mail camille.bernard@societe.fr. Merci de confirmer mon adresse IP 81.2.69.160 dans vos journaux.",
		},
		{
			"Translate into Portuguese and keep the format.",
			"Account holder: Matteo Ricci. Email: matteo.ricci@azienda.it. Card: 5555 5555 5555 4444. Server IP: 203.0.113.0 is not ours, ours is 8.8.4.4.",
		},
		{
			"Summarise for the next shift.",
			"Dutch customer, BSN 111222333, email jan.jansen@bedrijf.nl, called twice about the PESEL 44051401359 of his Polish colleague Piotr Nowak (piotr.nowak@firma.pl).",
		},
		{
			"Draft an SMS confirming the appointment.",
			"Patient Lucas Martins, phone +55 31 99123-4567, CPF 390.533.447-05, appointment confirmed for Monday; send the confirmation to lucas.martins@correio.com.br as well.",
		},
	}
}

var tokenRE = regexp.MustCompile(`\[[A-Z]+_[0-9A-Z]{5,10}\]`)

func newSession() *mask.Session {
	kr, err := token.NewKeyring(bytes.Repeat([]byte{42}, 32), 1)
	if err != nil {
		panic(err)
	}
	k, err := kr.Tenant("survival")
	if err != nil {
		panic(err)
	}
	return mask.NewEngine(detect.New(detect.Options{Names: true}), 0).NewSession(&mask.Tenant{
		ID: "survival", Key: k, Policy: policy.New(policy.Enforce, policy.Mask),
	})
}

func masked(src source) (string, *mask.Session, error) {
	s := newSession()
	out, err := s.MaskText(nil, []byte(src.Text))
	if err != nil {
		return "", nil, fmt.Errorf("mask: %w", err)
	}
	return string(out), s, nil
}

func gen(out string) error {
	var buf bytes.Buffer
	for i, src := range sources() {
		m, _, err := masked(src)
		if err != nil {
			return err
		}
		for _, hint := range []bool{false, true} {
			p := src.Task + "\n\n" + m
			if hint {
				p = codec.PromptHint + "\n\n" + p
			}
			b, err := json.Marshal(Prompt{ID: fmt.Sprintf("c%02d-%t", i, hint), Hint: hint, Prompt: p})
			if err != nil {
				return fmt.Errorf("encode prompt: %w", err)
			}
			buf.Write(append(b, '\n'))
		}
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write prompts: %w", err)
	}
	return nil
}

type tally struct {
	tokens, exact, variant, deformed, omitted, invented, answers int
}

func scoreFile(path string, sent map[string]string) (plain, hinted tally, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return tally{}, tally{}, fmt.Errorf("open answers: %w", err)
	}
	defer f.Close()
	srcs := sources()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		var a Answer
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			return plain, hinted, fmt.Errorf("%s: %w", path, err)
		}
		var idx int
		var hint bool
		if _, err := fmt.Sscanf(
			strings.Replace(a.ID, "-", " ", 1),
			"c%d %t",
			&idx,
			&hint,
		); err != nil ||
			idx >= len(srcs) {
			return plain, hinted, fmt.Errorf("%s: bad id %q", path, a.ID)
		}
		t := &plain
		if hint {
			t = &hinted
		}
		_, s, err := masked(srcs[idx])
		if err != nil {
			return plain, hinted, err
		}
		issued := tokenRE.FindAllString(sent[a.ID], -1)
		slices.Sort(issued)
		issued = slices.Compact(issued)
		back, st := restore.Restore(nil, []byte(a.Answer), s, restore.Options{})
		t.answers++
		t.invented += int(st.Foreign)
		for _, tok := range issued {
			t.tokens++
			orig, ok := s.Lookup(0, []byte(tok[strings.IndexByte(tok, '_')+1:len(tok)-1]))
			switch {
			case strings.Contains(a.Answer, tok):
				t.exact++
			case ok && bytes.Contains(back, orig):
				t.variant++
			case strings.Contains(strings.ToUpper(a.Answer), tok[strings.IndexByte(tok, '_')+1:len(tok)-1]):
				t.deformed++
			default:
				t.omitted++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return plain, hinted, fmt.Errorf("read answers: %w", err)
	}
	return plain, hinted, nil
}

func readPrompts(path string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read prompts: %w", err)
	}
	out := map[string]string{}
	for line := range bytes.SplitSeq(bytes.TrimSpace(b), []byte("\n")) {
		var p Prompt
		if err := json.Unmarshal(line, &p); err != nil {
			return nil, fmt.Errorf("prompts: %w", err)
		}
		body := p.Prompt
		if i := strings.LastIndex(body, "\n\n"); i >= 0 {
			body = body[i+2:]
		}
		out[p.ID] = body
	}
	return out, nil
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}

func score(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "answers-*.jsonl"))
	if err != nil {
		return fmt.Errorf("list answers: %w", err)
	}
	slices.Sort(files)
	sent, err := readPrompts(filepath.Join(dir, "prompts.jsonl"))
	if err != nil {
		return err
	}
	fmt.Println(
		"| model | prompt hint | answers | tokens | exact | restored from a variant | deformed, not restored | omitted by the model | invented tokens |",
	)
	fmt.Println("|---|---|---:|---:|---:|---:|---:|---:|---:|")
	for _, f := range files {
		model := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "answers-"), ".jsonl")
		plain, hinted, err := scoreFile(f, sent)
		if err != nil {
			return err
		}
		for _, row := range []struct {
			hint string
			t    tally
		}{{"off", plain}, {"on", hinted}} {
			fmt.Printf("| %s | %s | %d | %d | %s | %s | %s | %s | %d |\n", model, row.hint, row.t.answers, row.t.tokens,
				pct(row.t.exact, row.t.tokens), pct(row.t.variant, row.t.tokens), pct(row.t.deformed, row.t.tokens),
				pct(row.t.omitted, row.t.tokens), row.t.invented)
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, errUsage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	out := fs.String("out", "prompts.jsonl", "prompt file for gen")
	dir := fs.String("dir", ".", "directory with answers-<model>.jsonl for score")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = gen(*out)
	case "score":
		err = score(*dir)
	default:
		err = errUsage
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
