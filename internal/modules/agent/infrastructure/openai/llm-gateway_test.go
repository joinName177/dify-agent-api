package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
)

func collect(events <-chan ports.StreamEvent, errs <-chan error) ([]ports.StreamEvent, error) {
	var got []ports.StreamEvent
	for event := range events {
		got = append(got, event)
	}
	return got, <-errs
}

func TestChatStreamParsesSSEAndSendsExpectedRequest(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"Hel"}}]}

data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}

data: {"usage":{"prompt_tokens":10,"completion_tokens":5}}

data: [DONE]

`)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL+"/", "deepseek-chat")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})

	got, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", gotAuth)
	}
	if gotBody["model"] != "deepseek-chat" || gotBody["stream"] != true {
		t.Errorf("request body = %v, want model deepseek-chat with stream enabled", gotBody)
	}
	want := []ports.StreamEvent{
		{Type: "text_delta", Content: "Hel"},
		{Type: "text_delta", Content: "lo"},
		{Type: "done", Usage: &ports.UsageInfo{PromptTokens: 10, CompletionTokens: 5}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestChatStreamYieldsErrorEventOnNonOKStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", http.StatusBadGateway)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-chat")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})

	got, streamErr := collect(events, errs)
	if streamErr == nil {
		t.Fatal("expected a stream error for non-OK status")
	}

	if len(got) != 1 || got[0].Type != "error" {
		t.Fatalf("events = %+v, want a single error event", got)
	}
}

func TestChatStreamSendsDoneOnFinishReason(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-chat")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})

	got, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	want := []ports.StreamEvent{
		{Type: "text_delta", Content: "Hello"},
		{Type: "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestChatStreamSendsDoneOnEmptyContent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-chat")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})

	got, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	want := []ports.StreamEvent{
		{Type: "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestChatStreamSkipsUnparseableSSEChunks(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: not-json\n\n")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"OK"}}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-chat")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})

	got, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	want := []ports.StreamEvent{{Type: "text_delta", Content: "OK"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestChatStreamOverridesModelFromRequest(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"OK"},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-v4-flash")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
		Model:    "deepseek-v4-pro",
	})

	_, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	if gotBody["model"] != "deepseek-v4-pro" {
		t.Errorf("model = %q, want deepseek-v4-pro", gotBody["model"])
	}
}

func TestChatStreamInjectsSearchResults(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"Based on search results..."},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()

	gateway := NewLLMGateway("sk-test", server.URL, "deepseek-v4-flash")
	events, errs := gateway.ChatStream(context.Background(), ports.ChatStreamRequest{
		Messages:      []ports.ChatMessage{{Role: "user", Content: "What is new?"}},
		SearchResults: "1. News\n   Latest updates\n   https://example.com",
	})

	_, streamErr := collect(events, errs)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}

	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %v", messages)
	}
	first, ok := messages[0].(map[string]any)
	if !ok || first["role"] != "system" {
		t.Errorf("first message should be system with search results, got %v", first)
	}
	if s, ok := first["content"].(string); !ok || !strings.Contains(s, "News") {
		t.Errorf("system message should contain search results, got %q", s)
	}
}
