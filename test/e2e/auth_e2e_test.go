//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	allowedOrigin = "https://app.example.test"
	adminPassword = "integration-admin-password"
)

type envelope struct {
	Code      int             `json:"code"`
	ErrorCode string          `json:"errorCode"`
	RequestID string          `json:"requestId"`
	Data      json.RawMessage `json:"data"`
}

type authData struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
}

func TestAuthenticationJourney(t *testing.T) {
	identitySuffix := time.Now().UTC().UnixNano()
	adminEmail := fmt.Sprintf("admin-%d@example.test", identitySuffix)
	client, baseURL := startAPI(t, adminEmail)

	for _, endpoint := range []string{"livez", "readyz", "version"} {
		response := request(t, client, http.MethodGet, baseURL+"/"+endpoint, nil, "", "")
		assertEnvelope(t, response, http.StatusOK, "")
	}

	email := fmt.Sprintf("journey-%d@example.test", identitySuffix)
	registration := map[string]any{"email": email, "password": "integration-user-password", "displayName": "Integration User"}
	registered := assertAuth(t, request(t, client, http.MethodPost, baseURL+"/auth/register", registration, allowedOrigin, ""), http.StatusCreated, email, "user")

	assertEnvelope(t, request(t, client, http.MethodPost, baseURL+"/auth/register", registration, allowedOrigin, ""), http.StatusConflict, "EMAIL_ALREADY_REGISTERED")
	assertEnvelope(t, request(t, client, http.MethodPost, baseURL+"/auth/login", map[string]any{"email": email, "password": "wrong-password"}, allowedOrigin, ""), http.StatusUnauthorized, "INVALID_CREDENTIALS")
	assertAuth(t, request(t, client, http.MethodPost, baseURL+"/auth/login", map[string]any{"email": email, "password": "integration-user-password"}, allowedOrigin, ""), http.StatusOK, email, "user")
	oldRefreshCookie := refreshCookie(t, client, baseURL)

	me := request(t, client, http.MethodGet, baseURL+"/auth/me", nil, "", registered.AccessToken)
	assertEnvelope(t, me, http.StatusOK, "")

	rotated := assertAuth(t, request(t, client, http.MethodPost, baseURL+"/auth/refresh", nil, allowedOrigin, ""), http.StatusOK, email, "user")
	if rotated.AccessToken == registered.AccessToken {
		t.Fatal("refresh did not rotate the access token")
	}
	replayClient := newClient(t)
	replayRequest, err := http.NewRequest(http.MethodPost, baseURL+"/auth/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	replayRequest.Header.Set("Origin", allowedOrigin)
	replayRequest.AddCookie(oldRefreshCookie)
	replayResponse, err := replayClient.Do(replayRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertEnvelope(t, replayResponse, http.StatusUnauthorized, "UNAUTHENTICATED")

	assertEnvelope(t, request(t, client, http.MethodPost, baseURL+"/auth/logout", nil, allowedOrigin, ""), http.StatusOK, "")
	assertEnvelope(t, request(t, client, http.MethodGet, baseURL+"/auth/me", nil, "", rotated.AccessToken), http.StatusUnauthorized, "UNAUTHENTICATED")

	attackerEmail := fmt.Sprintf("attacker-%d@example.test", identitySuffix)
	privilegeInput := map[string]any{"email": attackerEmail, "password": "integration-user-password", "displayName": "Attacker", "role": "super_admin"}
	assertEnvelope(t, request(t, client, http.MethodPost, baseURL+"/auth/register", privilegeInput, allowedOrigin, ""), http.StatusBadRequest, "MALFORMED_JSON")
	assertEnvelope(t, request(t, client, http.MethodPost, baseURL+"/auth/login", map[string]any{"email": attackerEmail, "password": "integration-user-password"}, allowedOrigin, ""), http.StatusUnauthorized, "INVALID_CREDENTIALS")
	assertAuth(t, request(t, client, http.MethodPost, baseURL+"/auth/login", map[string]any{"email": adminEmail, "password": adminPassword}, allowedOrigin, ""), http.StatusOK, adminEmail, "super_admin")
}

func startAPI(t *testing.T, adminEmail string) (*http.Client, string) {
	t.Helper()
	databaseURL := os.Getenv("DIFY_AGENT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DIFY_AGENT_DATABASE_URL is required")
	}
	binary := os.Getenv("E2E_API_BINARY")
	if binary == "" {
		t.Fatal("E2E_API_BINARY is required; run make e2e-test")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(binary)
	command.Env = append(withoutServiceEnvironment(os.Environ()),
		"DIFY_AGENT_ENVIRONMENT=test",
		"DIFY_AGENT_HTTP_ADDRESS="+address,
		"DIFY_AGENT_DATABASE_URL="+databaseURL,
		"DIFY_AGENT_DATABASE_MIN_CONNS=0",
		"DIFY_AGENT_DATABASE_MAX_CONNS=5",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL="+adminEmail,
		"DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD="+adminPassword,
		"DIFY_AGENT_CORS_ALLOWED_ORIGINS="+allowedOrigin,
		"DIFY_AGENT_RELEASE_ID=e2e",
		"DIFY_AGENT_BUILD_COMMIT=e2e",
	)
	var processOutput bytes.Buffer
	command.Stdout, command.Stderr = &processOutput, &processOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Signal(os.Interrupt)
		select {
		case err := <-waitResult:
			if err != nil {
				t.Errorf("API process did not exit cleanly: %v; process output withheld", err)
			}
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			<-waitResult
			t.Error("API process did not stop within the shutdown timeout")
		}
	})

	client := newClient(t)
	baseURL := "http://" + address + "/api/v1"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return client, baseURL
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("API process did not become ready; process output withheld")
	return nil, ""
}

func withoutServiceEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		if !strings.HasPrefix(item, "DIFY_AGENT_") && !strings.HasPrefix(item, "E2E_API_BINARY=") {
			result = append(result, item)
		}
	}
	return result
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

func request(t *testing.T, client *http.Client, method, endpoint string, payload any, origin, bearer string) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertEnvelope(t *testing.T, response *http.Response, status int, errorCode string) envelope {
	t.Helper()
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("status=%d want=%d; response body withheld to protect credentials", response.StatusCode, status)
	}
	var decoded envelope
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatalf("decode response: %v; response body withheld", err)
	}
	if decoded.Code != status || decoded.ErrorCode != errorCode || decoded.RequestID == "" {
		t.Fatalf("invalid response envelope: code=%d errorCode=%q requestIDPresent=%t", decoded.Code, decoded.ErrorCode, decoded.RequestID != "")
	}
	return decoded
}

func assertAuth(t *testing.T, response *http.Response, status int, email, role string) authData {
	t.Helper()
	decoded := assertEnvelope(t, response, status, "")
	var data authData
	if err := json.Unmarshal(decoded.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.AccessToken == "" || data.User.Email != email || data.User.Role != role {
		t.Fatalf("invalid auth data: accessTokenPresent=%t email=%q role=%q", data.AccessToken != "", data.User.Email, data.User.Role)
	}
	return data
}

func refreshCookie(t *testing.T, client *http.Client, baseURL string) *http.Cookie {
	t.Helper()
	parsed, err := url.Parse(baseURL + "/auth/refresh")
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "dify_agent_refresh" {
			copy := *cookie
			return &copy
		}
	}
	t.Fatal("refresh cookie missing")
	return nil
}

func TestNoProductionSecretInFixture(t *testing.T) {
	for _, value := range []string{adminPassword, allowedOrigin} {
		if strings.Contains(value, ".com") || strings.Contains(value, ".net") || strings.Contains(value, ".org") {
			t.Fatalf("E2E fixture must use reserved test domains: %q", value)
		}
	}
}
