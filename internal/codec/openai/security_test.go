package openai

import (
	"strings"
	"testing"
)

func maskBody(t *testing.T, body string) string {
	t.Helper()
	out, err := New().MaskRequest(nil, []byte(body), session(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRequestBypassesAreMasked(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"escaped messages key": `{"messages":[{"role":"user","content":"cpf 529.982.247-25"}]}`,
		"numeric tool arg": `{"messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function",` +
			`"function":{"name":"pay","arguments":"{\"card\":4242424242424242,\"cpf\":52998224725}"}}]}]}`,
		"safety identifier": `{"messages":[],"safety_identifier":"ana.souza@correio.pt"}`,
		"metadata":          `{"messages":[],"metadata":{"customer":"ana.souza@correio.pt"}}`,
		"prediction":        `{"messages":[],"prediction":{"type":"content","content":"cpf 529.982.247-25"}}`,
		"tool description":  `{"messages":[],"tools":[{"type":"function","function":{"name":"x","description":"mail ana.souza@correio.pt"}}]}`,
		"legacy function_call": `{"messages":[{"role":"assistant","function_call":{"name":"x",` +
			`"arguments":"{\"to\":\"ana.souza@correio.pt\"}"}}]}`,
		"message name": `{"messages":[{"role":"user","name":"ana.souza@correio.pt","content":"hi"}]}`,
		"refusal":      `{"messages":[{"role":"assistant","refusal":"no, ana.souza@correio.pt"}]}`,
		"http url":     `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x.pt/?u=ana.souza@correio.pt"}}]}]}`,
	}
	for name, body := range cases {
		out := maskBody(t, body)
		for _, leak := range []string{"529.982.247-25", "52998224725", "4242424242424242", "ana.souza@correio.pt"} {
			if strings.Contains(out, leak) {
				t.Errorf("%s: %q leaked in %s", name, leak, out)
			}
		}
	}
}

func TestRequestStructureIsPreserved(t *testing.T) {
	t.Parallel()
	body := `{"model":"gpt-4.1-mini","messages":[{"role":"user","content":[{"type":"image_url","image_url":` +
		`{"url":"data:image/png;base64,QUJDREVG52998224725"}}]}],"tools":[{"type":"function","function":` +
		`{"name":"send_mail_52998224725","parameters":{"type":"object","properties":{"to":{"type":"string",` +
		`"enum":["52998224725"]}}}}}],"response_format":{"type":"json_schema"},"tool_choice":"auto"}`
	if out := maskBody(t, body); out != body {
		t.Fatalf("structural fields must not change:\n%s\n%s", body, out)
	}
}
