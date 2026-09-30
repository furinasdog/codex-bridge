package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codex-bridge/codex-bridge/internal/anthropic"
	"github.com/codex-bridge/codex-bridge/internal/config"
	"github.com/codex-bridge/codex-bridge/internal/openai"
)

type testToken struct{}

func (testToken) Token(context.Context) (string, error) { return "test-token", nil }

func TestMessagesEndToEnd(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization header was not forwarded")
		}
		body, _ := io.ReadAll(request.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["store"] != false || payload["stream"] != true || payload["model"] != "gpt-test" {
			t.Fatalf("unexpected upstream payload: %s", body)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		_, _ = writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = writer.Write([]byte("data: {\"type\":\"response.output_text.done\"}\n\n"))
		_, _ = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":4,\"output_tokens\":1}}}\n\n"))
	}))
	defer upstream.Close()

	cfg := config.Config{APIKey: "bridge-secret", AllowedOrigins: []string{"https://example.test"}}
	client := openai.NewClient(upstream.URL, testToken{}, upstream.Client(), "test")
	handler := anthropic.NewHandler(client, "gpt-test", 1<<20, time.Minute)
	router := NewRouter(cfg, handler)
	body := []byte(`{"model":"claude-test","max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", "bridge-secret")
	request.Header.Set("Origin", "https://example.test")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://example.test" {
		t.Fatalf("expected allowed CORS origin")
	}
	var response anthropic.MessageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "msg_resp_1" || response.Model != "claude-test" || response.Content[0].Text != "hello" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestNonLoopbackRequiresAPIKey(t *testing.T) {
	t.Parallel()
	if isLoopbackAddress("0.0.0.0:8787") {
		t.Fatal("wildcard address must not be treated as loopback")
	}
	if !isLoopbackAddress("127.0.0.1:8787") {
		t.Fatal("loopback address was not recognized")
	}
}
