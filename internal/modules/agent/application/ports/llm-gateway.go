package ports

import "context"

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string // user, assistant, or system
	Content string
}

// ToolDefinition describes a callable tool available to the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// UsageInfo holds token-usage metadata returned by the LLM provider.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// StreamEvent represents one SSE event in the chat stream.
type StreamEvent struct {
	Type    string     `json:"type"`              // text_delta, done, error
	Content string     `json:"content,omitempty"` // incremental text chunk
	Usage   *UsageInfo `json:"usage,omitempty"`   // final token counts
}

// ChatStreamRequest bundles the inputs for a streaming chat completion call.
type ChatStreamRequest struct {
	Messages      []ChatMessage
	Model         string
	SearchResults string
	Tools         []ToolDefinition
	WebSearch     bool
}

// LLMGateway is the port for an OpenAI-compatible streaming chat API.
type LLMGateway interface {
	ChatStream(ctx context.Context, req ChatStreamRequest) (<-chan StreamEvent, <-chan error)
}
