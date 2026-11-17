package app

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

type geminiRequest struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

func fakeGemini(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(string(body), "ana.souza@correio.pt") || r.Header.Get("x-goog-api-key") != "g-key" {
			http.Error(w, "leak or missing key", http.StatusTeapot)
			return
		}
		var req geminiRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		echo := req.Contents[0].Parts[0].Text
		if strings.HasSuffix(r.URL.Path, ":streamGenerateContent") && r.URL.Query().Get("alt") == "sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < len(echo); i += 5 {
				part := jsonx.Quote(nil, []byte(echo[i:min(i+5, len(echo))]))
				fmt.Fprintf(
					w,
					"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%s}],\"role\":\"model\"},\"index\":0}]}\r\n\r\n",
					part,
				)
			}
			fmt.Fprint(
				w,
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}]}\r\n\r\n",
			)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(
			w,
			`{"candidates":[{"content":{"parts":[{"text":%s}],"role":"model"},"finishReason":"STOP","index":0}]}`,
			jsonx.Quote(nil, []byte(echo)),
		)
	}))
}

func TestGeminiRoute(t *testing.T) {
	t.Parallel()
	up := fakeGemini(t)
	defer up.Close()
	cfg := config.Defaults()
	cfg.Keys.MasterKey = strings.Repeat("ab", 32)
	cfg.Audit.Enabled = false
	cfg.Routes = []config.Route{{Name: "gemini", Path: "/v1beta/models/", Upstream: up.URL, Codec: "gemini"}}
	cfg.Tenants = []config.Tenant{{ID: "acme", KeySHA256: config.HashKey(tenantKey)}}
	a, err := New(cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	prompt := `{"contents":[{"role":"user","parts":[{"text":"Write to ana.souza@correio.pt today"}]}]}`
	for _, target := range []string{
		"/v1beta/models/gemini-2.5-flash:generateContent",
		"/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse",
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(prompt))
		req.Header.Set(keyHeader, tenantKey)
		req.Header.Set("x-goog-api-key", "g-key")
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", target, rec.Code, rec.Body)
		}
		if !strings.Contains(joinTexts(t, rec.Body.String()), "Write to ana.souza@correio.pt today") {
			t.Fatalf("%s: not restored:\n%s", target, rec.Body)
		}
	}
}

func joinTexts(t *testing.T, body string) string {
	t.Helper()
	var b strings.Builder
	for line := range strings.SplitSeq(body, "\n") {
		data := strings.TrimPrefix(strings.TrimRight(line, "\r"), "data: ")
		var ev struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		for _, c := range ev.Candidates {
			for _, p := range c.Content.Parts {
				b.WriteString(p.Text)
			}
		}
	}
	return b.String()
}
