package httputil

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, errorCode, safeMessage string) {
	WriteJSON(w, status, ErrorResponse{Code: status, ErrorCode: errorCode, Message: safeMessage, RequestID: RequestIDFromContext(r.Context())})
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
