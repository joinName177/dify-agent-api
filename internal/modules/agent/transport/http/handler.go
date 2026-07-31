package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/usecases"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
)

// Handler processes agent chat HTTP requests.
type Handler struct {
	chatUseCase *usecases.ChatUseCase
	logger      *slog.Logger
}

// NewHandler creates an agent HTTP handler.
func NewHandler(chatUseCase *usecases.ChatUseCase, logger *slog.Logger) *Handler {
	return &Handler{chatUseCase: chatUseCase, logger: logger}
}

// ChatWithAgent handles POST /agent/chat as an SSE stream.
func (h *Handler) ChatWithAgent(w http.ResponseWriter, r *http.Request) {
	identity, err := platformauth.IdentityFromContext(r.Context())
	if err != nil {
		platformhttp.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	h.logger.InfoContext(r.Context(), "chat request", "user_id", identity.PublicID)

	var body openapiv1.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		platformhttp.WriteError(w, r, http.StatusBadRequest, "BAD_REQUEST", "The request body is malformed.")
		return
	}
	if len(body.Messages) == 0 {
		platformhttp.WriteError(w, r, http.StatusBadRequest, "BAD_REQUEST", "messages must not be empty.")
		return
	}

	req := ports.ChatStreamRequest{
		Messages: make([]ports.ChatMessage, 0, len(body.Messages)),
	}
	for _, msg := range body.Messages {
		req.Messages = append(req.Messages, ports.ChatMessage{Role: string(msg.Role), Content: msg.Content})
	}
	if body.Tools != nil {
		req.Tools = make([]ports.ToolDefinition, 0, len(*body.Tools))
		for _, t := range *body.Tools {
			req.Tools = append(req.Tools, ports.ToolDefinition{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
		}
	}

	events, errs, err := h.chatUseCase.Chat(r.Context(), req)
	if err != nil {
		platformhttp.WriteError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		platformhttp.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Streaming is not supported.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for event := range events {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		buf, err := json.Marshal(event)
		if err != nil {
			h.logger.ErrorContext(r.Context(), "failed to marshal sse event", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", buf); err != nil {
			return
		}
		flusher.Flush()
	}

	select {
	case chErr := <-errs:
		if chErr != nil {
			h.logger.ErrorContext(r.Context(), "chat stream error", "error", chErr)
		}
	default:
	}
}
