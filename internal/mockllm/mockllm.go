package mockllm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	maxBody      = 8 << 20
	roleAsst     = "assistant"
	evDelta      = "content_block_delta"
	evStart      = "content_block_start"
	evStop       = "content_block_stop"
	chunkObject  = "chat.completion.chunk"
	echoTool     = "echo"
	defaultModel = "mock-1"
	typeText     = "text"
	callID       = "call_mock"
)

type Options struct {
	ChunkMin   int
	ChunkMax   int
	ChunkDelay time.Duration
	Latency    time.Duration
	Dim        int
	Seed       uint64
	Log        *slog.Logger
}

type Server struct {
	opts  Options
	mu    sync.Mutex
	last  []byte
	state uint64
}

func New(o Options) *Server {
	o.ChunkMin = max(o.ChunkMin, 1)
	o.ChunkMax = max(o.ChunkMax, o.ChunkMin)
	if o.Dim <= 0 {
		o.Dim = 1536
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return &Server{opts: o, state: o.Seed | 1}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.chat)
	mux.HandleFunc("POST /v1/messages", s.messages)
	mux.HandleFunc("POST /v1/responses", s.responses)
	mux.HandleFunc("POST /v1/embeddings", s.embeddings)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.countTokens)
	mux.HandleFunc("GET /v1/models", s.models)
	mux.HandleFunc("GET /last", s.lastBody)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		s.write(w, []byte("ok\n"))
	})
	return mux
}

func (s *Server) write(w io.Writer, b []byte) {
	if _, err := w.Write(b); err != nil {
		s.opts.Log.Debug("mockllm write failed", slog.String("error", err.Error()))
	}
}

func (s *Server) next() uint64 {
	s.state ^= s.state << 13
	s.state ^= s.state >> 7
	s.state ^= s.state << 17
	return s.state
}

func (s *Server) record(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	s.mu.Lock()
	s.last = body
	s.mu.Unlock()
	if s.opts.Latency > 0 {
		time.Sleep(s.opts.Latency)
	}
	return body, nil
}

func (s *Server) lastBody(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	b := s.last
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if len(b) == 0 {
		b = []byte("{}")
	}
	s.write(w, b)
}

func (s *Server) chunks(text string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	span := s.opts.ChunkMax - s.opts.ChunkMin + 1
	var out []string
	for i := 0; i < len(text); {
		n := s.opts.ChunkMin + int(s.next()&0x7fffffff)%span
		end := min(i+n, len(text))
		for end < len(text) && text[end]&0xC0 == 0x80 {
			end++
		}
		out = append(out, text[i:end])
		i = end
	}
	return out
}

type message struct {
	Role    string         `json:"role"`
	Content jsontext.Value `json:"content"`
}

type part struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func contentText(v jsontext.Value) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var parts []part
	if err := json.Unmarshal(v, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == typeText {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func lastUser(msgs []message) string {
	for _, msg := range slices.Backward(msgs) {
		if msg.Role == "user" {
			return contentText(msg.Content)
		}
	}
	return ""
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	s.write(w, b)
}

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Server) badRequest(w http.ResponseWriter, err error) {
	var e apiError
	e.Error.Type, e.Error.Message = "invalid_request", err.Error()
	s.writeJSON(w, http.StatusBadRequest, e)
}

type chatRequest struct {
	Model    string           `json:"model"`
	Stream   bool             `json:"stream"`
	Messages []message        `json:"messages"`
	Tools    []jsontext.Value `json:"tools"`
}

type function struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitzero"`
}

type toolCall struct {
	Index    *int      `json:"index,omitzero"`
	ID       string    `json:"id,omitempty"`
	Type     string    `json:"type,omitempty"`
	Function *function `json:"function,omitzero"`
}

type chatMessage struct {
	Role      string     `json:"role,omitempty"`
	Content   *string    `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitzero"`
}

type delta struct {
	Role      string     `json:"role,omitempty"`
	Content   *string    `json:"content,omitzero"`
	ToolCalls []toolCall `json:"tool_calls,omitzero"`
}

type choice struct {
	Index        int          `json:"index"`
	Message      *chatMessage `json:"message,omitzero"`
	Delta        *delta       `json:"delta,omitzero"`
	FinishReason *string      `json:"finish_reason"`
}

type chatUsage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Total      int `json:"total_tokens"`
}

type completion struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []choice   `json:"choices"`
	Usage   *chatUsage `json:"usage,omitzero"`
}

//go:fix inline
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	body, err := s.record(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.badRequest(w, err)
		return
	}
	text := lastUser(req.Messages)
	reply := "I received: " + text
	toolArgs := ""
	if len(req.Tools) > 0 || r.Header.Get("X-Mock-Mode") == "toolcall" {
		args, err := json.Marshal(map[string]string{typeText: text})
		if err != nil {
			s.badRequest(w, err)
			return
		}
		toolArgs = string(args)
	}
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	if req.Stream {
		s.streamChat(w, r, model, reply, toolArgs)
		return
	}
	msg := &chatMessage{Role: roleAsst, Content: new(reply)}
	finish := "stop"
	if toolArgs != "" {
		msg.Content = nil
		msg.ToolCalls = []toolCall{
			{ID: callID, Type: "function", Function: &function{Name: echoTool, Arguments: new(toolArgs)}},
		}
		finish = "tool_calls"
	}
	s.writeJSON(w, http.StatusOK, completion{
		ID: "chatcmpl-mock", Object: "chat.completion", Created: time.Now().Unix(), Model: model,
		Choices: []choice{{Message: msg, FinishReason: new(finish)}},
		Usage:   &chatUsage{Prompt: len(body) / 4, Completion: len(reply) / 4, Total: (len(body) + len(reply)) / 4},
	})
}

type sseWriter struct {
	srv *Server
	w   http.ResponseWriter
	rc  *http.ResponseController
	d   time.Duration
}

func (s *Server) newSSE(w http.ResponseWriter) *sseWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	return &sseWriter{srv: s, w: w, rc: http.NewResponseController(w), d: s.opts.ChunkDelay}
}

func (o *sseWriter) raw(b string) {
	o.srv.write(o.w, []byte(b))
	if err := o.rc.Flush(); err != nil {
		o.srv.opts.Log.Debug("mockllm flush failed", slog.String("error", err.Error()))
	}
}

func (o *sseWriter) send(event string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	prefix := ""
	if event != "" {
		prefix = "event: " + event + "\n"
	}
	o.raw(prefix + "data: " + string(b) + "\n\n")
	if o.d > 0 {
		time.Sleep(o.d)
	}
}

func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, model, reply, toolArgs string) {
	out := s.newSSE(w)
	created := time.Now().Unix()
	chunk := func(d *delta, finish *string) completion {
		return completion{
			ID: "chatcmpl-mock", Object: chunkObject, Created: created, Model: model,
			Choices: []choice{{Delta: d, FinishReason: finish}},
		}
	}
	out.send("", chunk(&delta{Role: roleAsst, Content: new("")}, nil))
	finish := "stop"
	if toolArgs != "" {
		finish = "tool_calls"
		out.send("", chunk(&delta{ToolCalls: []toolCall{{
			Index: new(0), ID: callID, Type: "function", Function: &function{Name: echoTool, Arguments: new("")},
		}}}, nil))
		for _, c := range s.chunks(toolArgs) {
			if r.Context().Err() != nil {
				return
			}
			out.send(
				"",
				chunk(&delta{ToolCalls: []toolCall{{Index: new(0), Function: &function{Arguments: new(c)}}}}, nil),
			)
		}
	} else {
		for _, c := range s.chunks(reply) {
			if r.Context().Err() != nil {
				return
			}
			out.send("", chunk(&delta{Content: new(c)}, nil))
		}
	}
	out.send("", chunk(&delta{}, new(finish)))
	out.raw("data: [DONE]\n\n")
}

type anthropicRequest struct {
	Model    string         `json:"model"`
	Stream   bool           `json:"stream"`
	Messages []message      `json:"messages"`
	Tools    jsontext.Value `json:"tools"`
}

type block struct {
	Type     string            `json:"type"`
	Text     *string           `json:"text,omitzero"`
	Thinking *string           `json:"thinking,omitzero"`
	ID       string            `json:"id,omitempty"`
	Name     string            `json:"name,omitempty"`
	Input    map[string]string `json:"input,omitzero"`
}

type anthUsage struct {
	Input  int `json:"input_tokens,omitzero"`
	Output int `json:"output_tokens"`
}

type anthMessage struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Role         string     `json:"role"`
	Model        string     `json:"model"`
	Content      []block    `json:"content"`
	StopReason   *string    `json:"stop_reason"`
	StopSequence *string    `json:"stop_sequence"`
	Usage        *anthUsage `json:"usage,omitzero"`
}

type anthDelta struct {
	Type         string  `json:"type,omitempty"`
	Text         *string `json:"text,omitzero"`
	Thinking     *string `json:"thinking,omitzero"`
	Signature    *string `json:"signature,omitzero"`
	PartialJSON  *string `json:"partial_json,omitzero"`
	StopReason   *string `json:"stop_reason,omitzero"`
	StopSequence *string `json:"stop_sequence,omitzero"`
}

type anthEvent struct {
	Type         string       `json:"type"`
	Index        *int         `json:"index,omitzero"`
	Message      *anthMessage `json:"message,omitzero"`
	ContentBlock *block       `json:"content_block,omitzero"`
	Delta        *anthDelta   `json:"delta,omitzero"`
	Usage        *anthUsage   `json:"usage,omitzero"`
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	body, err := s.record(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.badRequest(w, err)
		return
	}
	text := lastUser(req.Messages)
	reply := "I received: " + text
	withTool := len(req.Tools) > 0 && string(req.Tools) != "null"
	if !req.Stream {
		content := []block{{Type: typeText, Text: new(reply)}}
		if withTool {
			content = append(
				content,
				block{Type: "tool_use", ID: "toolu_mock", Name: echoTool, Input: map[string]string{typeText: text}},
			)
		}
		s.writeJSON(w, http.StatusOK, anthMessage{
			ID: msgID, Type: itemMessage, Role: roleAsst, Model: req.Model, Content: content,
			StopReason: new("end_turn"), Usage: &anthUsage{Input: len(body) / 4, Output: len(reply) / 4},
		})
		return
	}
	s.streamMessages(w, r, req.Model, len(body), text, reply, withTool)
}

func (s *Server) streamMessages(
	w http.ResponseWriter,
	r *http.Request,
	model string,
	bodyLen int,
	text, reply string,
	withTool bool,
) {
	out := s.newSSE(w)
	out.send("message_start", anthEvent{Type: "message_start", Message: &anthMessage{
		ID: msgID, Type: itemMessage, Role: roleAsst, Model: model, Content: []block{},
		Usage: &anthUsage{Input: bodyLen / 4, Output: 1},
	}})
	out.send(
		evStart,
		anthEvent{Type: evStart, Index: new(0), ContentBlock: &block{Type: "thinking", Thinking: new("")}},
	)
	out.send(evDelta, anthEvent{Type: evDelta, Index: new(0), Delta: &anthDelta{
		Type: "thinking_delta", Thinking: new("[EMAIL_ZZZZZZ] stays opaque in thinking"),
	}})
	out.send(
		evDelta,
		anthEvent{
			Type:  evDelta,
			Index: new(0),
			Delta: &anthDelta{Type: "signature_delta", Signature: new("mock-signature")},
		},
	)
	out.send(evStop, anthEvent{Type: evStop, Index: new(0)})
	out.send(evStart, anthEvent{Type: evStart, Index: new(1), ContentBlock: &block{Type: typeText, Text: new("")}})
	for _, c := range s.chunks(reply) {
		if r.Context().Err() != nil {
			return
		}
		out.send(evDelta, anthEvent{Type: evDelta, Index: new(1), Delta: &anthDelta{Type: "text_delta", Text: new(c)}})
	}
	out.send(evStop, anthEvent{Type: evStop, Index: new(1)})
	if withTool {
		args, err := json.Marshal(map[string]string{typeText: text})
		if err == nil {
			out.send(evStart, anthEvent{Type: evStart, Index: new(2), ContentBlock: &block{
				Type: "tool_use", ID: "toolu_mock", Name: echoTool, Input: map[string]string{},
			}})
			for _, c := range s.chunks(string(args)) {
				out.send(
					evDelta,
					anthEvent{
						Type:  evDelta,
						Index: new(2),
						Delta: &anthDelta{Type: "input_json_delta", PartialJSON: new(c)},
					},
				)
			}
			out.send(evStop, anthEvent{Type: evStop, Index: new(2)})
		}
	}
	out.send(
		"message_delta",
		anthEvent{
			Type:  "message_delta",
			Delta: &anthDelta{StopReason: new("end_turn")},
			Usage: &anthUsage{Output: len(reply) / 4},
		},
	)
	out.send("message_stop", anthEvent{Type: "message_stop"})
}

type embeddingRequest struct {
	Model string         `json:"model"`
	Input jsontext.Value `json:"input"`
}

type embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type embeddingResponse struct {
	Object string      `json:"object"`
	Data   []embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  struct {
		Prompt int `json:"prompt_tokens"`
		Total  int `json:"total_tokens"`
	} `json:"usage"`
}

func inputs(v jsontext.Value) []string {
	var one string
	if err := json.Unmarshal(v, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(v, &many); err == nil {
		return many
	}
	return nil
}

func (s *Server) Embed(text string) []float64 {
	vec := make([]float64, s.opts.Dim)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '[' && r != ']'
	})
	for _, wd := range words {
		h := fnv.New64a()
		h.Write([]byte(wd))
		sum := h.Sum64()
		idx := int(sum>>33) % len(vec)
		if sum&(1<<63) != 0 {
			vec[idx]--
		} else {
			vec[idx]++
		}
	}
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm == 0 {
		vec[0] = 1
		return vec
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

type modelList struct {
	Object string      `json:"object"`
	Data   []modelInfo `json:"data"`
}

type modelInfo struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

func (s *Server) models(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, modelList{Object: "list", Data: []modelInfo{{ID: defaultModel, Object: "model"}}})
}

type tokenCount struct {
	InputTokens int `json:"input_tokens"`
}

func (s *Server) countTokens(w http.ResponseWriter, r *http.Request) {
	body, err := s.record(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, tokenCount{InputTokens: len(body) / 4})
}

func (s *Server) embeddings(w http.ResponseWriter, r *http.Request) {
	body, err := s.record(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	var req embeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.badRequest(w, err)
		return
	}
	in := inputs(req.Input)
	resp := embeddingResponse{Object: "list", Model: req.Model, Data: make([]embedding, len(in))}
	for i, t := range in {
		resp.Data[i] = embedding{Object: "embedding", Index: i, Embedding: s.Embed(t)}
	}
	resp.Usage.Prompt, resp.Usage.Total = 1, 1
	s.writeJSON(w, http.StatusOK, resp)
}
