package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

func TestClientStream(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("unexpected authorization header")
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		_, _ = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"status\":\"completed\"}}\n\n"))
	}))
	defer server.Close()
	client := NewClient(server.URL, staticToken("token"), server.Client(), "test")
	var types []string
	err := client.Stream(context.Background(), ResponseRequest{Model: "model", Stream: true}, func(event StreamEvent) error {
		types = append(types, event.Type)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "response.output_text.delta,response.completed" {
		t.Fatalf("unexpected events: %v", types)
	}
}

func TestClientObservesResponseHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Codex-Plan-Type", "pro")
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\"}}\n\n"))
	}))
	defer server.Close()
	client := NewClient(server.URL, staticToken("token"), server.Client(), "test")
	observed := make(chan string, 1)
	client.SetResponseObserver(func(header http.Header) { observed <- header.Get("X-Codex-Plan-Type") })
	if err := client.Stream(context.Background(), ResponseRequest{}, func(StreamEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := <-observed; got != "pro" {
		t.Fatalf("observed plan = %q, want pro", got)
	}
}

func TestClientObservesRateLimitEvents(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"type\":\"codex.rate_limits\",\"plan_type\":\"pro\",\"rate_limits\":{\"primary\":{\"used_percent\":20,\"window_minutes\":300}},\"credits\":{\"has_credits\":false,\"unlimited\":false,\"balance\":\"0\"}}\n\n"))
		_, _ = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\"}}\n\n"))
	}))
	defer server.Close()
	client := NewClient(server.URL, staticToken("token"), server.Client(), "test")
	observed := make(chan StreamEvent, 1)
	client.SetEventObserver(func(event StreamEvent) {
		if event.Type == "codex.rate_limits" {
			observed <- event
		}
	})
	if err := client.Stream(context.Background(), ResponseRequest{}, func(StreamEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	event := <-observed
	if event.PlanType != "pro" || event.RateLimits == nil || event.RateLimits.Primary == nil || event.RateLimits.Primary.UsedPercent == nil || *event.RateLimits.Primary.UsedPercent != 20 {
		t.Fatalf("unexpected rate-limit event: %#v", event)
	}
}

func TestClientRequiresCompletedEvent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
	}))
	defer server.Close()
	client := NewClient(server.URL, staticToken("token"), server.Client(), "test")
	if err := client.Stream(context.Background(), ResponseRequest{}, func(StreamEvent) error { return nil }); err == nil {
		t.Fatal("Stream() succeeded without response.completed")
	}
}
