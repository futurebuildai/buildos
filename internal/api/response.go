package api

import (
	"encoding/json"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// envelope wraps all API responses in the standard format.
type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
	Meta  meta      `json:"meta"`
}

type meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type apiError struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []fieldError `json:"details,omitempty"`
}

type fieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Data: data,
		Meta: buildMeta(r),
	})
}

func writeErrorResponse(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Error: &apiError{Code: code, Message: message},
		Meta:  buildMeta(r),
	})
}

func writeNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeErrorResponse(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", "this endpoint is not yet implemented")
}

func buildMeta(r *http.Request) meta {
	reqID := chimw.GetReqID(r.Context())
	return meta{
		RequestID: reqID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
