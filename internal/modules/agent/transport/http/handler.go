package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/infrastructure/search"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
)

// Handler processes agent chat HTTP requests.
type Handler struct {
	chatUseCase   *usecases.ChatUseCase
	logger        *slog.Logger
	searchGateway search.Gateway
}

// NewHandler creates an agent HTTP handler.
func NewHandler(chatUseCase *usecases.ChatUseCase, logger *slog.Logger, searchGateway search.Gateway) *Handler {
	return &Handler{chatUseCase: chatUseCase, logger: logger, searchGateway: searchGateway}
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
	if body.Model != nil {
		req.Model = *body.Model
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

	if body.WebSearch != nil && *body.WebSearch && h.searchGateway != nil {
		query := extractSearchQuery(req.Messages)
		if query != "" {
			results, searchErr := h.searchGateway.Search(r.Context(), query)
			if searchErr != nil {
				h.logger.WarnContext(r.Context(), "web search failed", "error", searchErr)
			} else {
				req.SearchResults = formatSearchResults(results)
				req.WebSearch = true
			}
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

	eventCount := 0
	for event := range events {
		eventCount++
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

	// Ensure the stream always terminates with a done event, even when the
	// underlying gateway produced no events (e.g. LLM returned an empty response).
	if eventCount == 0 {
		buf, err := json.Marshal(ports.StreamEvent{Type: "done"})
		if err == nil {
			fmt.Fprintf(w, "data: %s\n\n", buf)
			flusher.Flush()
		}
	}

	select {
	case chErr := <-errs:
		if chErr != nil {
			h.logger.ErrorContext(r.Context(), "chat stream error", "error", chErr)
		}
	default:
	}
}

// extractSearchQuery returns the content of the last user message as a search query.
func extractSearchQuery(messages []ports.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

// formatSearchResults formats search results as a plain-text summary.
func formatSearchResults(results []search.Result) string {
	var b strings.Builder
	for i, r := range results {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.Snippet, r.URL)
	}
	return strings.TrimSpace(b.String())
}
