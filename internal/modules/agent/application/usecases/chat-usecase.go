package usecases

import (
	"context"
	"errors"
	"slices"

	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
)

var allowedRoles = []string{"user", "assistant", "system"}

// ChatUseCase orchestrates a streaming chat request.
type ChatUseCase struct {
	gateway ports.LLMGateway
}

// NewChatUseCase creates a ChatUseCase backed by the given LLM gateway.
func NewChatUseCase(gateway ports.LLMGateway) *ChatUseCase {
	return &ChatUseCase{gateway: gateway}
}

// Chat validates the input, delegates to the LLM, and returns an event channel.
// The caller must read both the event and error channels to avoid goroutine leaks.
func (uc *ChatUseCase) Chat(ctx context.Context, req ports.ChatStreamRequest) (<-chan ports.StreamEvent, <-chan error, error) {
	if len(req.Messages) == 0 {
		return nil, nil, errors.New("messages must not be empty")
	}
	for i, msg := range req.Messages {
		if msg.Content == "" {
			return nil, nil, errors.New("message content must not be empty")
		}
		if !slices.Contains(allowedRoles, msg.Role) {
			return nil, nil, errors.New("message role must be user, assistant, or system")
		}
		_ = i // reserved for future validation
	}
	events, errs := uc.gateway.ChatStream(ctx, req)
	return events, errs, nil
}
