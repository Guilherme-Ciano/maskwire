package detect

import (
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

func secretsIn(d *Detector, in string) []string {
	var out []string
	for _, sp := range d.Detect(nil, []byte(in)) {
		if sp.Type == entity.Secret {
			out = append(out, in[sp.Start:sp.End])
		}
	}
	return out
}

func TestSecrets(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0Z3VS5JJcds3xfn\n-----END RSA PRIVATE KEY-----"
	stripeKey := "sk_" + "live_" + strings.Repeat("Ab3dE9", 4)
	slackToken := "xox" + "b-123456789012-abcdefABCDEF"
	cases := map[string]string{
		"key AKIAIOSFODNN7EXAMPLE in env":                         "AKIAIOSFODNN7EXAMPLE",
		"token ghp_" + strings.Repeat("a1B2", 9) + " leaked":      "ghp_" + strings.Repeat("a1B2", 9),
		"stripe " + stripeKey + " ok":                             stripeKey,
		"use sk-ant-api03-" + strings.Repeat("Ab9_", 10) + " now": "sk-ant-api03-" + strings.Repeat("Ab9_", 10),
		"maps AIzaSyA" + strings.Repeat("b", 32) + " here":        "AIzaSyA" + strings.Repeat("b", 32),
		"slack " + slackToken + " ok":                             slackToken,
		"Authorization: Bearer " + jwt:                            jwt,
		"Password: Tr0ub4dor&3xyz! ok":                            "Tr0ub4dor&3xyz!",
		`{"api_key": "9f86d081884c7d659a2feaa0c55ad015"}`:         "9f86d081884c7d659a2feaa0c55ad015",
		"key:\n" + pem + "\nend":                                  pem,
	}
	for in, want := range cases {
		got := secretsIn(d, in)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	for _, in := range []string{
		"password: please reset it tomorrow",
		"the token was expired",
		"ask the secret santa",
		"sk-short",
		"eyJ alone",
		"ghp_tooShort",
	} {
		if got := secretsIn(d, in); len(got) != 0 {
			t.Errorf("%q: unexpected secret %q", in, got)
		}
	}
	if got := secretsIn(New(Options{DisableSecrets: true}), "AKIAIOSFODNN7EXAMPLE"); len(got) != 0 {
		t.Errorf("secrets detected while disabled: %q", got)
	}
}

func TestSecretBeforeDashTerminates(t *testing.T) {
	t.Parallel()
	d := New(Options{})
	for _, text := range []string{
		"key AKIAIOSFODNN7EXAMPLE- end",
		"AKIAIOSFODNN7EXAMPLE-",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789-x",
		"token npm_abcdefghijklmnopqrstuvwxyz0123456789--",
	} {
		spans := d.Detect(nil, []byte(text))
		if len(spans) != 1 || spans[0].Type != entity.Secret {
			t.Errorf("%q: %+v", text, spans)
		}
	}
}
