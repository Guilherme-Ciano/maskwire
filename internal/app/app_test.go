package app

import (
	"bufio"
	"encoding/json/v2"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/mockllm"
)

const (
	tenantKey = "demo-key"
	prompt    = "Escreva para ana.silva@empresa.com.br (CPF 529.982.247-25), IBAN PT50 0002 0123 1234 5678 9015 4."
	chatPath  = "/v1/chat/completions"
	keyHeader = "X-Maskwire-Key"
)

var leaks = []string{"ana.silva", "529.982.247-25", "PT50 0002"}

type harness struct {
	t      *testing.T
	proxy  *httptest.Server
	mock   *httptest.Server
	cache  *httptest.Server
	app    *App
	cfg    config.Config
	cached []byte
}

type result struct {
	status int
	header http.Header
	body   string
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	t.Helper()
	h := &harness{t: t}
	h.mock = httptest.NewServer(mockllm.New(mockllm.Options{ChunkMin: 1, ChunkMax: 7, Seed: 42}).Handler())
	t.Cleanup(h.mock.Close)
	h.cache = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(h.cached); err != nil {
			t.Errorf("cache stub write: %v", err)
		}
	}))
	t.Cleanup(h.cache.Close)
	cfg := config.Defaults()
	cfg.Keys.MasterKey = strings.Repeat("ab", 32)
	cfg.Audit.Dir = t.TempDir()
	cfg.Server.StreamIdleTimeout = 0
	cfg.Routes = []config.Route{
		{Name: "openai", Path: chatPath, Upstream: h.mock.URL, Codec: "openai"},
		{Name: "anthropic", Path: "/v1/messages", Upstream: h.mock.URL, Codec: "anthropic"},
		{Name: "direct", Path: "/direct" + chatPath, Upstream: h.mock.URL + chatPath, Codec: "openai"},
		{Name: "cache", Path: "/cache" + chatPath, Upstream: h.cache.URL, Codec: "openai", SynthesizeStream: true},
	}
	cfg.Tenants = []config.Tenant{{
		ID: "acme", KeySHA256: config.HashKey(tenantKey),
		ByType: map[string]config.Rule{"CREDIT_CARD": {Action: "block"}},
	}}
	if mutate != nil {
		mutate(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	a, err := New(cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	h.app, h.cfg = a, cfg
	done := make(chan error, 1)
	go func() { done <- a.auditor.Run(t.Context()) }()
	t.Cleanup(func() {
		if err := <-done; err != nil {
			t.Errorf("audit writer: %v", err)
		}
	})
	h.proxy = httptest.NewServer(a.Handler())
	t.Cleanup(h.proxy.Close)
	return h
}

func (h *harness) do(method, url, body string, hdr map[string]string) result {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), method, url, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return result{status: resp.StatusCode, header: resp.Header, body: string(b)}
}

func (h *harness) post(path, body string, extra map[string]string) result {
	h.t.Helper()
	hdr := map[string]string{"Content-Type": "application/json", keyHeader: tenantKey}
	maps.Copy(hdr, extra)
	return h.do(http.MethodPost, h.proxy.URL+path, body, hdr)
}

func (h *harness) upstreamSaw(allowRaw bool) string {
	h.t.Helper()
	saw := h.do(http.MethodGet, h.mock.URL+"/last", "", nil).body
	if !allowRaw {
		for _, l := range leaks {
			if strings.Contains(saw, l) {
				h.t.Fatalf("upstream received raw PII %q: %s", l, saw)
			}
		}
	}
	return saw
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func chatBody(t *testing.T, stream bool) string {
	t.Helper()
	return mustJSON(t, map[string]any{
		"model": "gpt-4.1-mini", "stream": stream,
		"messages": []any{map[string]string{"role": "user", "content": prompt}},
	})
}

type sseEvent struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

func sseText(t *testing.T, raw string) (content, tools string) {
	t.Helper()
	var c, tl strings.Builder
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("bad event %q: %v", data, err)
		}
		for _, ch := range ev.Choices {
			c.WriteString(ch.Delta.Content)
			for _, tc := range ch.Delta.ToolCalls {
				tl.WriteString(tc.Function.Arguments)
			}
		}
		if ev.Delta.Type == "text_delta" {
			c.WriteString(ev.Delta.Text)
		}
		tl.WriteString(ev.Delta.PartialJSON)
	}
	return c.String(), tl.String()
}

func TestOpenAINonStreaming(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post(chatPath, chatBody(t, false), nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "I received: "+prompt) {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	if r.header.Get("X-Maskwire-Masked") != "3" || r.header.Get("X-Maskwire-Restored") != "3" {
		t.Fatalf("headers %v", r.header)
	}
	saw := h.upstreamSaw(false)
	for _, want := range []string{"[EMAIL_", "[CPF_", "[IBAN_"} {
		if !strings.Contains(saw, want) {
			t.Fatalf("upstream body missing %s: %s", want, saw)
		}
	}
}

func TestUpstreamPathOverride(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post("/direct"+chatPath, chatBody(t, false), nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "I received: "+prompt) {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
}

func TestOpenAIStreaming(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post(chatPath, chatBody(t, true), nil)
	if ct := r.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	if content, _ := sseText(t, r.body); content != "I received: "+prompt {
		t.Fatalf("content %q", content)
	}
	h.upstreamSaw(false)
}

func TestOpenAIToolCallStreaming(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post(chatPath, chatBody(t, true), map[string]string{"X-Mock-Mode": "toolcall"})
	_, args := sseText(t, r.body)
	var parsed map[string]string
	if err := json.Unmarshal([]byte(args), &parsed); err != nil || parsed["text"] != prompt {
		t.Fatalf("args %q (%v)", args, err)
	}
}

func TestAnthropicStreamingAndJSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	body := `{"model":"claude-x","max_tokens":64,"stream":true,"system":"Be brief. Contact: ana.silva@empresa.com.br",` +
		`"tools":[{"name":"echo","input_schema":{"type":"object"}}],` +
		`"messages":[{"role":"user","content":[{"type":"text","text":` + mustJSON(
		t,
		prompt,
	) + `}]}]}`
	r := h.post("/v1/messages", body, nil)
	content, args := sseText(t, r.body)
	if content != "I received: "+prompt {
		t.Fatalf("content %q", content)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(args), &parsed); err != nil || parsed["text"] != prompt {
		t.Fatalf("args %q", args)
	}
	if !strings.Contains(r.body, "[EMAIL_ZZZZZZ] stays opaque in thinking") {
		t.Fatal("thinking deltas must pass through untouched")
	}
	h.upstreamSaw(false)
	r = h.post("/v1/messages", strings.Replace(body, `"stream":true`, `"stream":false`, 1), nil)
	q := mustJSON(t, prompt)
	if !strings.Contains(r.body, "I received: "+q[1:len(q)-1]) || !strings.Contains(r.body, `"input":{"text":`+q+`}`) {
		t.Fatalf("anthropic json %s", r.body)
	}
}

func TestBlockPolicy(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post(chatPath, strings.Replace(chatBody(t, false), "IBAN", "card 4242 4242 4242 4242, IBAN", 1), nil)
	if r.status != http.StatusUnprocessableEntity || !strings.Contains(r.body, `"entities":["CREDIT_CARD"]`) ||
		strings.Contains(r.body, "4242") {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
}

func TestAuthAndErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	cases := []struct {
		path, body string
		hdr        map[string]string
		want       int
	}{
		{chatPath, chatBody(t, false), map[string]string{keyHeader: "wrong"}, http.StatusUnauthorized},
		{chatPath, "{not json", nil, http.StatusBadRequest},
		{chatPath, `{"logprobs":true,"messages":[]}`, nil, http.StatusBadRequest},
		{chatPath, chatBody(t, false), map[string]string{"Content-Encoding": "gzip"}, http.StatusUnsupportedMediaType},
		{"/v1/other", "{}", nil, http.StatusNotFound},
	}
	for _, tc := range cases {
		if r := h.post(tc.path, tc.body, tc.hdr); r.status != tc.want {
			t.Errorf("%s %v: status %d want %d", tc.path, tc.hdr, r.status, tc.want)
		}
	}
}

func TestForeignTokenGuardAndSynthesizedStream(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	h.cached = []byte(`{"id":"scg","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,` +
		`"message":{"role":"assistant","content":"Reply to [EMAIL_AAAAAA] about [CPF_BBBBBB]"},"finish_reason":"stop"}]}`)
	r := h.post("/cache"+chatPath, chatBody(t, false), nil)
	if !strings.Contains(r.body, "Reply to [email] about [cpf]") {
		t.Fatalf("foreign tokens must be neutralised: %s", r.body)
	}
	r = h.post("/cache"+chatPath, chatBody(t, true), nil)
	content, _ := sseText(t, r.body)
	if !strings.HasPrefix(r.header.Get("Content-Type"), "text/event-stream") ||
		content != "Reply to [email] about [cpf]" {
		t.Fatalf("synthesized stream %q %v", content, r.header)
	}
}

func TestObserveMode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *config.Config) { c.Tenants[0].Mode = "observe" })
	r := h.post(chatPath, chatBody(t, false), nil)
	if !strings.Contains(r.body, "I received: "+prompt) {
		t.Fatalf("observe body %s", r.body)
	}
	if !strings.Contains(h.upstreamSaw(true), "ana.silva@empresa.com.br") {
		t.Fatal("observe mode must forward the original body")
	}
}

func TestAuditChainAfterTraffic(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	for range 3 {
		h.post(chatPath, chatBody(t, true), nil)
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.app.auditor.Written() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	rep, err := audit.VerifyDir(h.cfg.Audit.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Records < 3 || rep.Files != 1 {
		t.Fatalf("report %+v", rep)
	}
}

func TestAdminEndpoints(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	h.post(chatPath, chatBody(t, false), nil)
	srv := httptest.NewServer(h.app.AdminHandler())
	defer srv.Close()
	for path, want := range map[string]string{"/livez": "ok", "/readyz": "ready", "/metrics": "maskwire_detections_total"} {
		r := h.do(http.MethodGet, srv.URL+path, "", nil)
		if !strings.Contains(r.body, want) {
			t.Fatalf("%s: %s", path, r.body)
		}
		for _, l := range leaks {
			if strings.Contains(r.body, l) {
				t.Fatalf("%s leaks PII", path)
			}
		}
	}
}
