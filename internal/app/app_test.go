package app

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/mockllm"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const (
	tenantKey = "mw-demo-0b8e3c1f5a7d9e2b"
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
		{Name: "embeddings", Path: "/v1/embeddings", Upstream: h.mock.URL, Codec: "openai-embeddings"},
		{Name: "count", Path: "/v1/messages/count_tokens", Upstream: h.mock.URL, Codec: "anthropic"},
		{Name: "models", Path: "/v1/models", Upstream: h.mock.URL, Codec: "passthrough"},
		{Name: "responses", Path: "/v1/responses", Upstream: h.mock.URL, Codec: "openai-responses"},
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

func TestCompressedUpstreamIsDecoded(t *testing.T) {
	t.Parallel()
	var sawGzip atomic.Bool
	gz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		sawGzip.Store(strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		reply := `{"choices":[{"index":0,"message":{"role":"assistant","content":` + mustJSON(
			t,
			string(body),
		) + `},"finish_reason":"stop"}]}`
		if _, err := io.WriteString(zw, reply); err != nil {
			t.Error(err)
		}
		if err := zw.Close(); err != nil {
			t.Error(err)
		}
	}))
	defer gz.Close()
	h := newHarness(t, func(c *config.Config) {
		c.Routes = append(
			c.Routes,
			config.Route{Name: "gz", Path: "/gz" + chatPath, Upstream: gz.URL + chatPath, Codec: "openai"},
		)
	})
	r := h.post("/gz"+chatPath, chatBody(t, false), map[string]string{"Accept-Encoding": "br"})
	if r.status != http.StatusOK || !sawGzip.Load() {
		t.Fatalf("status %d gzip requested %v body %.200s", r.status, sawGzip.Load(), r.body)
	}
	if !strings.Contains(r.body, "ana.silva@empresa.com.br") || r.header.Get("Content-Encoding") != "" {
		t.Fatalf("compressed upstream body must be decoded and restored: %v %.300s", r.header, r.body)
	}
}

func TestPromptHint(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *config.Config) { c.Tenants[0].PromptHint = true })
	hint := "is a placeholder for personal data"
	h.post(chatPath, chatBody(t, false), nil)
	saw := h.upstreamSaw(false)
	if !strings.Contains(saw, `"messages":[{"role":"system","content":"Text such as`) {
		t.Fatalf("openai hint missing: %s", saw)
	}
	h.post(
		chatPath,
		mustJSON(
			t,
			map[string]any{
				"model":    "m",
				"messages": []any{map[string]string{"role": "user", "content": "no personal data here"}},
			},
		),
		nil,
	)
	if strings.Contains(h.upstreamSaw(false), hint) {
		t.Fatal("no hint without tokens")
	}
	h.post(
		"/v1/messages",
		`{"model":"c","max_tokens":5,"system":"Be brief.","messages":[{"role":"user","content":`+mustJSON(
			t,
			prompt,
		)+`}]}`,
		nil,
	)
	if saw := h.upstreamSaw(false); !strings.Contains(saw, hint) || !strings.Contains(saw, `Be brief."`) {
		t.Fatalf("anthropic hint: %s", saw)
	}
	h.post("/v1/responses", mustJSON(t, map[string]any{"model": "m", "input": prompt}), nil)
	if saw := h.upstreamSaw(false); !strings.Contains(saw, `"instructions":"Text such as`) {
		t.Fatalf("responses hint: %s", saw)
	}
}

func TestResponseScanCountsGeneratedPII(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	h.cached = []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":` +
		`"Call our office at +351 912 345 678 or mail help@company.pt"},"finish_reason":"stop"}]}`)
	h.post("/cache"+chatPath, chatBody(t, false), nil)
	h.post(chatPath, chatBody(t, false), nil)
	h.waitAudit(2)
	log := h.auditLog()
	if !strings.Contains(log, `"response_detections":{"EMAIL":1,"PHONE":1}`) {
		t.Fatalf("generated PII not counted: %s", log)
	}
	if strings.Count(log, `"response_detections":{}`) != 1 {
		t.Fatalf("echoed tokens must not count as generated PII: %s", log)
	}
	if strings.Contains(log, "company.pt") || strings.Contains(log, "912 345") {
		t.Fatal("audit must hold counts, never values")
	}
}

func TestEmbeddingsAreMasked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.post("/v1/embeddings", mustJSON(t, map[string]any{
		"model": "text-embedding-3-small",
		"input": []string{prompt, "second " + prompt},
	}), nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, `"embedding"`) {
		t.Fatalf("status %d body %.200s", r.status, r.body)
	}
	if saw := h.upstreamSaw(false); !strings.Contains(saw, "[EMAIL_") {
		t.Fatalf("embeddings input not masked: %s", saw)
	}
}

func TestCountTokensIsMasked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	body := `{"model":"claude-x","messages":[{"role":"user","content":` + mustJSON(t, prompt) + `}]}`
	r := h.post("/v1/messages/count_tokens", body, nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "input_tokens") {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	h.upstreamSaw(false)
}

func TestModelsPassthroughNeedsTenantKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	r := h.do(http.MethodGet, h.proxy.URL+"/v1/models", "", map[string]string{keyHeader: tenantKey})
	if r.status != http.StatusOK || !strings.Contains(r.body, `"mock-1"`) {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	if r := h.do(http.MethodGet, h.proxy.URL+"/v1/models", "", nil); r.status != http.StatusUnauthorized {
		t.Fatalf("models without key: status %d", r.status)
	}
	if r := h.post("/v1/models", "{}", nil); r.status == http.StatusOK {
		t.Fatal("passthrough route must not accept POST")
	}
}

type responsesStreamEvent struct {
	Type      string `json:"type"`
	Delta     string `json:"delta"`
	Text      string `json:"text"`
	Arguments string `json:"arguments"`
	Response  *struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	} `json:"response"`
}

func TestResponsesAPI(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	body := mustJSON(t, map[string]any{
		"model":        "gpt-5-mini",
		"instructions": "Reply to ana.silva@empresa.com.br",
		"tools": []any{
			map[string]any{"type": "function", "name": "echo", "parameters": map[string]any{"type": "object"}},
		},
		"input": []any{
			map[string]any{
				"type": "function_call", "call_id": "c0", "name": "lookup",
				"arguments": `{"cpf":11144477735,"mail":"ana.silva@empresa.com.br"}`,
			},
			map[string]any{"role": "user", "content": []any{map[string]string{"type": "input_text", "text": prompt}}},
		},
	})
	r := h.post("/v1/responses", body, nil)
	q := mustJSON(t, prompt)
	if r.status != http.StatusOK || !strings.Contains(r.body, "I received: "+q[1:len(q)-1]) ||
		!strings.Contains(r.body, `ana.silva@empresa.com.br`) {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	h.upstreamSaw(false)
	if saw := h.upstreamSaw(false); strings.Contains(saw, "11144477735") {
		t.Fatalf("numeric CPF in function_call arguments reached the upstream: %s", saw)
	}
	r = h.post(
		"/v1/responses",
		strings.Replace(body, `"model":"gpt-5-mini"`, `"model":"gpt-5-mini","stream":true`, 1),
		nil,
	)
	var text, args strings.Builder
	var doneText, completed string
	sc := bufio.NewScanner(strings.NewReader(r.body))
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("bad event %q: %v", data, err)
		}
		switch ev.Type {
		case "response.output_text.delta":
			text.WriteString(ev.Delta)
		case "response.function_call_arguments.delta":
			args.WriteString(ev.Delta)
		case "response.output_text.done":
			doneText = ev.Text
		case "response.completed":
			completed = ev.Response.Output[0].Content[0].Text + "|" + ev.Response.Output[1].Arguments
		}
	}
	want := "I received: " + prompt
	if text.String() != want || doneText != want {
		t.Fatalf("stream text %q done %q", text.String(), doneText)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(args.String()), &parsed); err != nil || parsed["text"] != prompt {
		t.Fatalf("stream args %q (%v)", args.String(), err)
	}
	if !strings.HasPrefix(completed, want+"|") || !strings.Contains(completed, "ana.silva@empresa.com.br") {
		t.Fatalf("completed event not restored: %q", completed)
	}
	h.upstreamSaw(false)
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

func TestStreamRestoredTrailer(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		h.proxy.URL+chatPath,
		strings.NewReader(chatBody(t, true)),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(keyHeader, tenantKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, ok := resp.Trailer["X-Maskwire-Restored"]; !ok {
		t.Fatalf("trailer not announced: %v", resp.Header)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if got := resp.Trailer.Get("X-Maskwire-Restored"); got != "3" {
		t.Fatalf("restored trailer %q", got)
	}
}

func TestTenantRateLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *config.Config) { c.Tenants[0].RateLimit = config.RateLimit{RPS: 0.01, Burst: 2} })
	for i := range 2 {
		if r := h.post(chatPath, chatBody(t, false), nil); r.status != http.StatusOK {
			t.Fatalf("request %d: %d", i, r.status)
		}
	}
	r := h.post(chatPath, chatBody(t, false), nil)
	if r.status != http.StatusTooManyRequests || r.header.Get("Retry-After") == "" {
		t.Fatalf("status %d retry %q", r.status, r.header.Get("Retry-After"))
	}
	if !strings.Contains(r.body, "rate_limited") {
		t.Fatalf("body %s", r.body)
	}
}

func TestOverheadMetric(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	h.post(chatPath, chatBody(t, true), nil)
	h.post(chatPath, chatBody(t, false), nil)
	srv := httptest.NewServer(h.app.AdminHandler())
	defer srv.Close()
	m := h.do(http.MethodGet, srv.URL+"/metrics", "", nil).body
	for _, want := range []string{
		`maskwire_overhead_seconds_count{route="openai",streaming="true"} 1`,
		`maskwire_overhead_seconds_count{route="openai",streaming="false"} 1`,
	} {
		if !strings.Contains(m, want) {
			t.Fatalf("missing %s", want)
		}
	}
}

func TestToolRestoreWithholdsTypes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *config.Config) { c.Tenants[0].ToolRestore = []string{"EMAIL"} })
	r := h.post(chatPath, chatBody(t, true), map[string]string{"X-Mock-Mode": "toolcall"})
	_, args := sseText(t, r.body)
	var parsed map[string]string
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Fatalf("args %q: %v", args, err)
	}
	text := parsed["text"]
	if !strings.Contains(text, "ana.silva@empresa.com.br") {
		t.Fatalf("email not restored: %q", text)
	}
	if strings.Contains(text, "529.982.247-25") || !strings.Contains(text, "[CPF_") {
		t.Fatalf("CPF restored in tool args: %q", text)
	}
	if content, _ := sseText(t, h.post(chatPath, chatBody(t, true), nil).body); content != "I received: "+prompt {
		t.Fatalf("text channel affected: %q", content)
	}
	srv := httptest.NewServer(h.app.AdminHandler())
	defer srv.Close()
	if m := h.do(
		http.MethodGet,
		srv.URL+"/metrics",
		"",
		nil,
	).body; !strings.Contains(
		m,
		`maskwire_tool_withheld_total{tenant="acme"} 2`,
	) {
		t.Fatalf("withheld metric missing")
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
		{chatPath, chatBody(t, false), map[string]string{keyHeader: "short"}, http.StatusUnauthorized},
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
	rep, err := audit.VerifyDir(h.cfg.Audit.Dir, h.sealer())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Records < 4 || rep.Files != 1 {
		t.Fatalf("report %+v", rep)
	}
}

func (h *harness) sealer() *audit.Sealer {
	h.t.Helper()
	master, err := hex.DecodeString(strings.Repeat("ab", 32))
	if err != nil {
		h.t.Fatal(err)
	}
	kr, err := token.NewKeyring(master, 1)
	if err != nil {
		h.t.Fatal(err)
	}
	key, err := kr.AuditKey()
	if err != nil {
		h.t.Fatal(err)
	}
	return audit.NewSealer(key)
}

func (h *harness) waitAudit(n uint64) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.app.auditor.Written() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.app.auditor.Written() < n {
		h.t.Fatalf("audit wrote %d records, want %d", h.app.auditor.Written(), n)
	}
}

func (h *harness) auditLog() string {
	h.t.Helper()
	files, err := filepath.Glob(filepath.Join(h.cfg.Audit.Dir, "*", "*.jsonl"))
	if err != nil || len(files) == 0 {
		h.t.Fatalf("no audit files: %v", err)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

func TestClientAbortMidStreamIsAudited(t *testing.T) {
	t.Parallel()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		rc := http.NewResponseController(w)
		for i := 0; r.Context().Err() == nil && i < 500; i++ {
			if _, err := io.WriteString(
				w,
				`data: {"choices":[{"index":0,"delta":{"content":"x"},"finish_reason":null}]}`+"\n\n",
			); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer slow.Close()
	h := newHarness(t, func(c *config.Config) {
		c.Routes = append(
			c.Routes,
			config.Route{Name: "slow", Path: "/slow" + chatPath, Upstream: slow.URL + chatPath, Codec: "openai"},
		)
	})
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.proxy.URL+"/slow"+chatPath,
		strings.NewReader(chatBody(t, true)),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(keyHeader, tenantKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = resp.Body.Close()
	h.waitAudit(1)
	if !strings.Contains(h.auditLog(), `"status":499`) {
		t.Fatalf("aborted stream not audited as 499: %s", h.auditLog())
	}
}

func TestUpstreamHeaderTimeoutIs504(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer hang.Close()
	defer close(release)
	h := newHarness(t, func(c *config.Config) {
		c.Server.UpstreamTimeout = 100 * time.Millisecond
		c.Routes = append(
			c.Routes,
			config.Route{Name: "hang", Path: "/hang" + chatPath, Upstream: hang.URL + chatPath, Codec: "openai"},
		)
	})
	r := h.post("/hang"+chatPath, chatBody(t, false), nil)
	if r.status != http.StatusGatewayTimeout {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	h.waitAudit(1)
	if !strings.Contains(h.auditLog(), `"status":504`) {
		t.Fatal("timeout not audited")
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
