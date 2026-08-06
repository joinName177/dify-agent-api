package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/agent/infrastructure/search"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
)

type fakeGateway struct {
	events []ports.StreamEvent
}

func (f *fakeGateway) ChatStream(_ context.Context, _ ports.ChatStreamRequest) (<-chan ports.StreamEvent, <-chan error) {
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

type fakeAuthenticator struct{}

func (fakeAuthenticator) Authenticate(_ context.Context, _ string) (platformauth.Identity, error) {
	return platformauth.Identity{PublicID: "u_test", Email: "person@example.test"}, nil
}

func newAuthedHandler(events []ports.StreamEvent, searchGateway search.Gateway) http.Handler {
	gateway := &fakeGateway{events: events}
	usecase := usecases.NewChatUseCase(gateway)
	handler := NewHandler(usecase, slog.New(slog.NewTextHandler(io.Discard, nil)), searchGateway)
	return platformauth.OptionalMiddleware(fakeAuthenticator{})(chatRouter{handler: handler})
}

// chatRouter adapts the plain Handler struct to http.Handler for tests.
type chatRouter struct {
	handler *Handler
}

func (r chatRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ChatWithAgent(w, req)
}

func authedRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

func TestChatWithAgentRequiresAuthentication(t *testing.T) {
	t.Parallel()
	handler := newAuthedHandler(nil, nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{"messages":[{"role":"user","content":"Hi"}]}`)))

	if res.Code != http.StatusUnauthorized || !strings.Contains(res.Body.String(), `"errorCode":"UNAUTHENTICATED"`) {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestChatWithAgentRejectsMalformedBody(t *testing.T) {
	t.Parallel()
	handler := newAuthedHandler(nil, nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, authedRequest(t, http.MethodPost, "/api/v1/agent/chat", "{not json"))

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"errorCode":"BAD_REQUEST"`) {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestChatWithAgentRejectsEmptyMessages(t *testing.T) {
	t.Parallel()
	handler := newAuthedHandler(nil, nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, authedRequest(t, http.MethodPost, "/api/v1/agent/chat", `{"messages":[]}`))

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "must not be empty") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestChatWithAgentStreamsSSEEvents(t *testing.T) {
	t.Parallel()
	handler := newAuthedHandler([]ports.StreamEvent{
		{Type: "text_delta", Content: "Hello"},
		{Type: "done", Usage: &ports.UsageInfo{PromptTokens: 10, CompletionTokens: 5}},
	}, nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, authedRequest(t, http.MethodPost, "/api/v1/agent/chat", `{"messages":[{"role":"user","content":"Hi"}]}`))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	body := res.Body.String()
	if !strings.Contains(body, `data: {"type":"text_delta","content":"Hello"}`) ||
		!strings.Contains(body, `data: {"type":"done","usage":{"prompt_tokens":10,"completion_tokens":5}}`) {
		t.Fatalf("SSE events missing from body: %s", body)
	}
}
