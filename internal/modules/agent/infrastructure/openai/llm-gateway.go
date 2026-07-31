package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
)

// LLMGateway implements ports.LLMGateway by calling an OpenAI-compatible streaming API.
type LLMGateway struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewLLMGateway creates an LLMGateway that targets the given provider.
func NewLLMGateway(apiKey, baseURL, model string) *LLMGateway {
	return &LLMGateway{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{},
	}
}

type openaiChatRequest struct {
	Model    string              `json:"model"`
	Messages []openaiChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content *string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

func (g *LLMGateway) ChatStream(ctx context.Context, req ports.ChatStreamRequest) (<-chan ports.StreamEvent, <-chan error) {
	events := make(chan ports.StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)
		if err := g.stream(ctx, req, events); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()

	return events, errs
}

func (g *LLMGateway) stream(ctx context.Context, req ports.ChatStreamRequest, events chan<- ports.StreamEvent) error {
	messages := make([]openaiChatMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, openaiChatMessage{Role: msg.Role, Content: msg.Content})
	}

	body := openaiChatRequest{
		Model:    g.model,
		Messages: messages,
		Stream:   true,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		events <- ports.StreamEvent{Type: "error", Content: "Failed to encode request."}
		return fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		events <- ports.StreamEvent{Type: "error", Content: "Failed to create HTTP request."}
		return fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		events <- ports.StreamEvent{Type: "error", Content: "LLM service is unreachable."}
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		safeBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		events <- ports.StreamEvent{Type: "error", Content: fmt.Sprintf("LLM service returned status %d.", resp.StatusCode)}
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(safeBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			events <- ports.StreamEvent{
				Type: "done",
				Usage: &ports.UsageInfo{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
				},
			}
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				events <- ports.StreamEvent{Type: "text_delta", Content: *choice.Delta.Content}
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		events <- ports.StreamEvent{Type: "error", Content: "Stream read failed."}
		return fmt.Errorf("scan sse stream: %w", err)
	}
	return nil
}
