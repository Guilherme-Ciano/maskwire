package mockllm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"slices"
	"strings"
	"time"
)

type responsesRequest struct {
	Model  string           `json:"model"`
	Stream bool             `json:"stream"`
	Input  jsontext.Value   `json:"input"`
	Tools  []jsontext.Value `json:"tools"`
}

type respInputItem struct {
	Role    string         `json:"role"`
	Content jsontext.Value `json:"content"`
}

type respPart struct {
	Type        string   `json:"type"`
	Text        *string  `json:"text,omitzero"`
	Annotations []string `json:"annotations,omitzero"`
}

type respItem struct {
	Type      string     `json:"type"`
	ID        string     `json:"id"`
	Status    string     `json:"status,omitempty"`
	Role      string     `json:"role,omitempty"`
	Content   []respPart `json:"content,omitzero"`
	CallID    string     `json:"call_id,omitempty"`
	Name      string     `json:"name,omitempty"`
	Arguments *string    `json:"arguments,omitzero"`
}

type respObject struct {
	ID        string     `json:"id"`
	Object    string     `json:"object"`
	CreatedAt int64      `json:"created_at"`
	Status    string     `json:"status"`
	Model     string     `json:"model"`
	Output    []respItem `json:"output"`
}

type respEvent struct {
	Type         string      `json:"type"`
	Response     *respObject `json:"response,omitzero"`
	OutputIndex  *int        `json:"output_index,omitzero"`
	ContentIndex *int        `json:"content_index,omitzero"`
	ItemID       string      `json:"item_id,omitempty"`
	Item         *respItem   `json:"item,omitzero"`
	Part         *respPart   `json:"part,omitzero"`
	Delta        *string     `json:"delta,omitzero"`
	Text         *string     `json:"text,omitzero"`
	Arguments    *string     `json:"arguments,omitzero"`
}

const (
	fcID             = "fc_mock"
	msgID            = "msg_mock"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
	itemMessage      = "message"
)

func responsesUserText(v jsontext.Value) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var items []respInputItem
	if err := json.Unmarshal(v, &items); err != nil {
		return ""
	}
	for _, item := range slices.Backward(items) {
		if item.Role != "user" {
			continue
		}
		var text string
		if err := json.Unmarshal(item.Content, &text); err == nil {
			return text
		}
		var parts []respPart
		if err := json.Unmarshal(item.Content, &parts); err != nil {
			return ""
		}
		var sb strings.Builder
		for _, p := range parts {
			if p.Text != nil {
				sb.WriteString(*p.Text)
			}
		}
		return sb.String()
	}
	return ""
}

func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	body, err := s.record(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.badRequest(w, err)
		return
	}
	text := responsesUserText(req.Input)
	reply := "I received: " + text
	msg := respItem{
		Type: itemMessage, ID: msgID, Status: statusCompleted, Role: roleAsst,
		Content: []respPart{{Type: "output_text", Text: new(reply), Annotations: []string{}}},
	}
	full := respObject{
		ID: "resp_mock", Object: "response", CreatedAt: time.Now().Unix(), Status: statusCompleted,
		Model: req.Model, Output: []respItem{msg},
	}
	args := ""
	if len(req.Tools) > 0 {
		b, err := json.Marshal(map[string]string{typeText: text})
		if err != nil {
			s.badRequest(w, err)
			return
		}
		args = string(b)
		full.Output = append(full.Output, respItem{
			Type: "function_call", ID: fcID, Status: statusCompleted,
			CallID: callID, Name: echoTool, Arguments: new(args),
		})
	}
	if !req.Stream {
		s.writeJSON(w, http.StatusOK, full)
		return
	}
	s.streamResponses(w, r, &full, reply, args)
}

func (s *Server) streamResponses(w http.ResponseWriter, r *http.Request, full *respObject, reply, args string) {
	out := s.newSSE(w)
	created := *full
	created.Status, created.Output = statusInProgress, []respItem{}
	out.send("response.created", respEvent{Type: "response.created", Response: &created})
	zero, one := 0, 1
	out.send("response.output_item.added", respEvent{
		Type:        "response.output_item.added",
		OutputIndex: &zero,
		Item: &respItem{
			Type:    itemMessage,
			ID:      msgID,
			Status:  statusInProgress,
			Role:    roleAsst,
			Content: []respPart{},
		},
	})
	for _, c := range s.chunks(reply) {
		if r.Context().Err() != nil {
			return
		}
		out.send("response.output_text.delta", respEvent{
			Type: "response.output_text.delta", ItemID: msgID,
			OutputIndex: &zero, ContentIndex: &zero, Delta: new(c),
		})
	}
	out.send("response.output_text.done", respEvent{
		Type: "response.output_text.done", ItemID: msgID,
		OutputIndex: &zero, ContentIndex: &zero, Text: new(reply),
	})
	out.send("response.output_item.done", respEvent{
		Type: "response.output_item.done", OutputIndex: &zero,
		Item: &full.Output[0],
	})
	if args != "" {
		out.send("response.output_item.added", respEvent{
			Type: "response.output_item.added", OutputIndex: &one,
			Item: &respItem{
				Type: "function_call", ID: fcID, Status: statusInProgress, CallID: callID,
				Name: echoTool, Arguments: new(""),
			},
		})
		for _, c := range s.chunks(args) {
			out.send("response.function_call_arguments.delta", respEvent{
				Type:   "response.function_call_arguments.delta",
				ItemID: fcID, OutputIndex: &one, Delta: new(c),
			})
		}
		out.send("response.function_call_arguments.done", respEvent{
			Type:   "response.function_call_arguments.done",
			ItemID: fcID, OutputIndex: &one, Arguments: new(args),
		})
		out.send("response.output_item.done", respEvent{
			Type: "response.output_item.done", OutputIndex: &one,
			Item: &full.Output[1],
		})
	}
	out.send("response.completed", respEvent{Type: "response.completed", Response: full})
}
