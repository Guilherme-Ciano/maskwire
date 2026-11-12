package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var errStatus = errors.New("unexpected status")

const chatPath = "/v1/chat/completions"

type client struct {
	proxy string
	mock  string
	key   string
	http  *http.Client
	out   io.Writer
}

type scenario struct {
	title  string
	path   string
	body   string
	stream bool
	anth   bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func chat(stream bool, text string) string {
	b, err := json.Marshal(map[string]any{
		"model": "gpt-4.1-mini", "stream": stream,
		"messages": []any{map[string]string{"role": "user", "content": text}},
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

func anthropic(text string) string {
	b, err := json.Marshal(map[string]any{
		"model":      "claude-sonnet-x",
		"max_tokens": 256,
		"stream":     true,
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]string{"type": "text", "text": text}}},
		},
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	proxy := fs.String("proxy", "http://localhost:8090", "maskwire base URL")
	mock := fs.String("mock", "http://localhost:19000", "mockllm base URL")
	key := fs.String("key", "demo-key", "tenant key")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	c := &client{proxy: *proxy, mock: *mock, key: *key, http: &http.Client{Timeout: 30 * time.Second}, out: out}
	ana := "Customer support ticket: please draft a short and polite reply to the customer at " +
		"ana.souza@correio.pt about the refund for order 7731, her CPF is 529.982.247-25 and the refund goes to " +
		"IBAN PT50 0002 0123 1234 5678 9015 4 within five working days."
	bruno := strings.NewReplacer("ana.souza@correio.pt", "bruno.lima@correio.pt", "529.982.247-25", "111.444.777-35").
		Replace(ana)
	scenarios := []scenario{
		{
			"1. OpenAI JSON through semantic-cache-gateway (expect cache MISS)",
			chatPath,
			chat(false, ana),
			false,
			false,
		},
		{
			"2. Same prompt, client asks for a stream (expect cache HIT, synthesized SSE)",
			chatPath,
			chat(true, ana),
			true,
			false,
		},
		{
			"3. Different customer, near-identical wording (cross-subject HIT, foreign-token guard)",
			chatPath,
			chat(false, bruno),
			false,
			false,
		},
		{
			"4. Direct route, real SSE split into 1-7 byte chunks",
			"/direct/v1/chat/completions",
			chat(true, ana),
			true,
			false,
		},
		{"5. Anthropic Messages, streaming with a thinking block", "/v1/messages", anthropic(ana), true, true},
		{
			"6. Block policy: card numbers never leave",
			chatPath,
			chat(false, "Charge card 4242 4242 4242 4242 for the customer."),
			false,
			false,
		},
	}
	ctx := context.Background()
	for _, s := range scenarios {
		if err := c.play(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

type reply struct {
	status int
	header http.Header
}

func (c *client) do(ctx context.Context, method, url, body string, auth bool) (reply, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return reply{}, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("X-Maskwire-Key", c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return reply{}, nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	r := reply{status: resp.StatusCode, header: resp.Header}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return r, nil, fmt.Errorf("read response: %w", err)
	}
	return r, b, nil
}

func (c *client) printf(format string, a ...any) {
	if _, err := fmt.Fprintf(c.out, format, a...); err != nil {
		return
	}
}

func (c *client) play(ctx context.Context, s scenario) error {
	c.printf("\n=== %s\n", s.title)
	c.printf("client sent    : %s\n", userText(s.body))
	resp, body, err := c.do(ctx, http.MethodPost, c.proxy+s.path, s.body, true)
	if err != nil {
		return err
	}
	c.printf(
		"status         : %d  masked=%s restored=%s cache=%s\n",
		resp.status,
		orDash(resp.header.Get("X-Maskwire-Masked")),
		orDash(resp.header.Get("X-Maskwire-Restored")),
		orDash(resp.header.Get("X-Semantic-Cache")),
	)
	if resp.status != http.StatusOK {
		c.printf("response       : %s", body)
		if resp.status == http.StatusUnprocessableEntity {
			return nil
		}
		return fmt.Errorf("%w: %d", errStatus, resp.status)
	}
	if resp.header.Get("X-Semantic-Cache") == "HIT" {
		c.printf("upstream saw   : (no upstream call, answered from the semantic cache)\n")
	} else if _, last, err := c.do(ctx, http.MethodGet, c.mock+"/last", "", false); err == nil {
		c.printf("upstream saw   : %s\n", userText(string(last)))
	}
	text, events := responseText(body, s.stream, s.anth)
	if s.stream {
		c.printf("stream events  : %d (content-type %s)\n", events, resp.header.Get("Content-Type"))
	}
	c.printf("client got     : %s\n", text)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

type anyMessage struct {
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
}

func userText(body string) string {
	var m anyMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil || len(m.Messages) == 0 {
		return body
	}
	switch v := m.Messages[len(m.Messages)-1].Content.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, p := range v {
			if part, ok := p.(map[string]any); ok {
				if t, ok := part["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return body
}

type completion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func responseText(body []byte, stream, anth bool) (text string, events int) {
	if !stream {
		var c completion
		if err := json.Unmarshal(body, &c); err != nil || len(c.Choices) == 0 {
			return string(body), 0
		}
		return c.Choices[0].Message.Content, 0
	}
	var sb strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		events++
		var c completion
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			continue
		}
		if anth && c.Delta.Type == "text_delta" {
			sb.WriteString(c.Delta.Text)
		}
		for _, ch := range c.Choices {
			sb.WriteString(ch.Delta.Content)
		}
	}
	return sb.String(), events
}
