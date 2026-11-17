package gemini

import (
	"fmt"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

type Codec struct {
	rw *jsonx.Rewriter
}

func New() *Codec {
	return &Codec{rw: jsonx.NewRewriter()}
}

func ParseMeta(body []byte) (codec.Meta, error) {
	if !jsonx.Valid(body) {
		return codec.Meta{}, codec.ErrInvalidBody
	}
	return codec.Meta{}, nil
}

func skipTop(name []byte) bool {
	switch string(name) {
	case "model", "safetySettings", "toolConfig", "cachedContent", "labels":
		return true
	}
	return false
}

func skipLeaf(name []byte) bool {
	switch string(name) {
	case "role", "mimeType", "fileUri", "thoughtSignature", "id", "language", "outcome":
		return true
	}
	return false
}

func underCall(path []jsonx.Frame) bool {
	return len(path) >= 7 && path[0].Is("contents") && path[2].Is("parts") &&
		(path[4].Is("functionCall") && path[5].Is("args") || path[4].Is("functionResponse") && path[5].Is("response"))
}

func generationConfig(path []jsonx.Frame) bool {
	if !path[0].Is("generationConfig") {
		return false
	}
	if len(path) >= 2 && path[1].Is("stopSequences") {
		return true
	}
	if root := codec.SchemaRoot(path, "responseSchema", "responseJsonSchema"); root > 0 {
		return codec.SchemaText(path, root)
	}
	return false
}

func toolDefinition(path []jsonx.Frame) (decided, mask bool) {
	if !path[0].Is("tools") {
		return false, false
	}
	if root := codec.SchemaRoot(
		path,
		"parameters",
		"parametersJsonSchema",
		"response",
		"responseJsonSchema",
	); root > 0 {
		return true, codec.SchemaText(path, root)
	}
	leaf := path[len(path)-1]
	return true, leaf.Is("description") && jsonx.Under(path, "functionDeclarations")
}

func requestTarget(path []jsonx.Frame, raw []byte) bool {
	if len(path) == 0 {
		return false
	}
	if !jsonx.IsString(raw) {
		return underCall(path)
	}
	if decided, mask := toolDefinition(path); decided {
		return mask
	}
	leaf := path[len(path)-1]
	switch {
	case skipTop(path[0].Name), path[0].Is("generationConfig") && !generationConfig(path):
		return false
	case path[0].Is("generationConfig"), underCall(path):
		return true
	case !leaf.Array && (skipLeaf(leaf.Name) || leaf.Is("name") && len(path) >= 2 && callName(path)):
		return false
	case jsonx.Under(path, "inlineData"), jsonx.Under(path, "fileData"), jsonx.LooksBase64(raw):
		return false
	}
	return true
}

func callName(path []jsonx.Frame) bool {
	parent := path[len(path)-2]
	return parent.Is("functionCall") || parent.Is("functionResponse")
}

func (c *Codec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		if !requestTarget(path, raw) {
			return dst, false, nil
		}
		if underCall(path) {
			return m.MaskLeafField(dst, jsonx.FieldName(path), raw)
		}
		return m.MaskLeaf(dst, raw)
	})
	if err != nil {
		return out, fmt.Errorf("mask gemini request: %w", err)
	}
	return out, nil
}

func partText(path []jsonx.Frame) bool {
	return len(path) == 6 && path[0].Is("candidates") && path[2].Is("content") && path[3].Is("parts") &&
		path[5].Is("text")
}

func callArgs(path []jsonx.Frame) bool {
	return len(path) >= 8 && path[0].Is("candidates") && path[2].Is("content") && path[3].Is("parts") &&
		path[5].Is("functionCall") && path[6].Is("args")
}

func (c *Codec) RestoreBody(dst, body []byte, set restore.Set, o restore.Options) ([]byte, restore.Stats, error) {
	var stats restore.Stats
	var scratch, restored []byte
	o.Encoding = restore.Raw
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		if !jsonx.IsString(raw) || !partText(path) && !callArgs(path) {
			return dst, false, nil
		}
		var err error
		scratch, err = jsonx.Unquote(scratch[:0], raw)
		if err != nil {
			return dst, false, err
		}
		opts := o
		opts.Tool = callArgs(path)
		var st restore.Stats
		restored, st = restore.Restore(restored[:0], scratch, set, opts)
		stats.Add(&st)
		if st.Restored == 0 && st.Foreign == 0 {
			return dst, false, nil
		}
		return jsonx.Quote(dst, restored), true, nil
	})
	if err != nil {
		return out, stats, fmt.Errorf("restore gemini body: %w", err)
	}
	return out, stats, nil
}

func Synthesize(dst, body []byte) ([]byte, error) {
	dst = append(dst, "data: "...)
	dst = append(dst, body...)
	return append(dst, "\n\n"...), nil
}

func InjectHint(doc []byte) ([]byte, error) {
	start, end, ok, err := jsonx.TopLevel(doc, "systemInstruction")
	if err != nil {
		return doc, fmt.Errorf("inject gemini hint: %w", err)
	}
	part := jsonx.Quote([]byte(`{"text":`), []byte(codec.PromptHint))
	part = append(part, '}')
	if !ok {
		out, err := jsonx.InsertTopLevel(
			doc,
			"systemInstruction",
			append(append([]byte(`{"parts":[`), part...), "]}"...),
		)
		if err != nil {
			return doc, fmt.Errorf("inject gemini hint: %w", err)
		}
		return out, nil
	}
	ps, pe, found, err := jsonx.TopLevel(doc[start:end], "parts")
	if err != nil || !found || doc[start+ps] != '[' {
		return doc, err
	}
	return jsonx.PrependToArray(doc, start+ps, start+pe, part), nil
}
