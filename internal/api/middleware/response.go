package middleware

import (
	"encoding/json"
	"net/http"
)

// errorResponse matches the API contract error format.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	if obs := errObserver; obs != nil {
		obs(code, statusClass(status))
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{Code: code, Message: message},
	})
}

// errObserver counts middleware-layer error responses (auth, rbac, setup-gate,
// rate-limit) by {code, status-class}. Set by the router via SetErrorObserver;
// nil when metrics are disabled.
var errObserver func(code, statusClass string)

// SetErrorObserver wires the error-response counter into this package's writer.
// Pass nil to disable.
func SetErrorObserver(fn func(code, statusClass string)) { errObserver = fn }

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}
