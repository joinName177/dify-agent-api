package httputil

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, safeMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Code: code, Message: safeMessage, RequestID: RequestIDFromContext(r.Context())})
}
