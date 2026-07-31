package usecases

import (
	"context"
	"reflect"
	"testing"

	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
)

type fakeGateway struct {
	events []ports.StreamEvent
	called bool
}

func (f *fakeGateway) ChatStream(_ context.Context, _ ports.ChatStreamRequest) (<-chan ports.StreamEvent, <-chan error) {
	f.called = true
	events := make(chan ports.StreamEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		for _, event := range f.events {
			events <- event
		}
	}()
	return events, errs
}

func TestChatValidatesInputBeforeCallingGateway(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  ports.ChatStreamRequest
	}{
		{"empty messages", ports.ChatStreamRequest{}},
		{"empty content", ports.ChatStreamRequest{Messages: []ports.ChatMessage{{Role: "user", Content: ""}}}},
		{"invalid role", ports.ChatStreamRequest{Messages: []ports.ChatMessage{{Role: "tool", Content: "hi"}}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gateway := &fakeGateway{}
			uc := NewChatUseCase(gateway)

			_, _, err := uc.Chat(context.Background(), test.req)

			if err == nil {
				t.Fatal("expected a validation error")
			}
			if gateway.called {
				t.Fatal("gateway must not be called for invalid input")
			}
		})
	}
}

func TestChatForwardsEventsFromGateway(t *testing.T) {
	t.Parallel()
	want := []ports.StreamEvent{
		{Type: "text_delta", Content: "Hello"},
		{Type: "done", Usage: &ports.UsageInfo{PromptTokens: 10, CompletionTokens: 5}},
	}
	gateway := &fakeGateway{events: want}
	uc := NewChatUseCase(gateway)

	events, errs, err := uc.Chat(context.Background(), ports.ChatStreamRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []ports.StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if streamErr := <-errs; streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}
