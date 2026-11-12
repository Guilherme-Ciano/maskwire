package httpapi

import (
	"net/http"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

func writeError(w http.ResponseWriter, status int, kind, msg string) {
	body := []byte(`{"error":{"type":`)
	body = jsonx.Quote(body, []byte(kind))
	body = append(body, `,"message":`...)
	body = jsonx.Quote(body, []byte(msg))
	body = append(body, "}}\n"...)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	writeBody(w, body)
}

func writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		return
	}
}

func writeBlocked(w http.ResponseWriter, types []string) {
	body := []byte(
		`{"error":{"type":"pii_blocked","message":"request contains entity types blocked by policy","entities":[`,
	)
	for i, t := range types {
		if i > 0 {
			body = append(body, ',')
		}
		body = jsonx.Quote(body, []byte(t))
	}
	body = append(body, "]}}\n"...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	writeBody(w, body)
}
