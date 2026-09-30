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
	"github.com/codex-bridge/codex-bridge/internal/models"
	"github.com/codex-bridge/codex-bridge/internal/openai"
	"github.com/codex-bridge/codex-bridge/internal/telemetry"
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
		if _, exists := payload["max_output_tokens"]; exists {
			t.Fatalf("Codex subscription endpoint does not accept max_output_tokens: %s", body)
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
	catalog := models.NewCatalog("gpt-fast", "gpt-test", "gpt-power")
	store := telemetry.New("")
	handler := anthropic.NewHandler(client, catalog, store, 1<<20, time.Minute)
	router := NewRouter(cfg, handler, store, catalog)
	body := []byte(`{"model":"gpt-test","max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`)

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
	if response.ID != "msg_resp_1" || response.Model != "gpt-test" || response.Content[0].Text != "hello" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestClaudeHelloProbe(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	client := openai.NewClient("http://127.0.0.1", testToken{}, nil, "test")
	catalog := models.NewCatalog("fast", "balanced", "powerful")
	store := telemetry.New("")
	handler := anthropic.NewHandler(client, catalog, store, 1<<20, time.Minute)
	router := NewRouter(cfg, handler, store, catalog)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, "/api/hello", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s /api/hello status = %d", method, recorder.Code)
		}
	}
}

func TestModelsAdvertisesConfiguredModels(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	client := openai.NewClient("http://127.0.0.1", testToken{}, nil, "test")
	catalog := models.NewCatalog("fast", "balanced", "powerful")
	store := telemetry.New("")
	handler := anthropic.NewHandler(client, catalog, store, 1<<20, time.Minute)
	router := NewRouter(cfg, handler, store, catalog)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/models status = %d", recorder.Code)
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	want := []string{"fast", "balanced", "powerful"}
	if len(response.Data) != len(want) {
		t.Fatalf("models = %#v, want %v", response.Data, want)
	}
	for index, id := range want {
		if response.Data[index].ID != id {
			t.Fatalf("models[%d] = %q, want %q", index, response.Data[index].ID, id)
		}
	}
}

func TestDashboardIsEmbeddedAndLocalOnly(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	client := openai.NewClient("http://127.0.0.1", testToken{}, nil, "test")
	catalog := models.NewCatalog("fast", "balanced", "powerful")
	store := telemetry.New("")
	handler := anthropic.NewHandler(client, catalog, store, 1<<20, time.Minute)
	router := NewRouter(cfg, handler, store, catalog)

	page := httptest.NewRecorder()
	pageRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	pageRequest.RemoteAddr = "127.0.0.1:1234"
	router.ServeHTTP(page, pageRequest)
	if page.Code != http.StatusOK || !bytes.Contains(page.Body.Bytes(), []byte("Codex Bridge")) {
		t.Fatalf("dashboard page status = %d, body = %q", page.Code, page.Body.String())
	}

	metrics := httptest.NewRecorder()
	metricsRequest := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	metricsRequest.RemoteAddr = "127.0.0.1:1234"
	router.ServeHTTP(metrics, metricsRequest)
	if metrics.Code != http.StatusOK || !bytes.Contains(metrics.Body.Bytes(), []byte(`"id":"balanced"`)) {
		t.Fatalf("dashboard API status = %d, body = %q", metrics.Code, metrics.Body.String())
	}

	forbidden := httptest.NewRecorder()
	forbiddenRequest := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	forbiddenRequest.RemoteAddr = "192.0.2.10:1234"
	router.ServeHTTP(forbidden, forbiddenRequest)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-loopback dashboard status = %d, want 403", forbidden.Code)
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
