package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/codex-bridge/codex-bridge/internal/openai"
)

type recordedEvent struct {
	name string
	data any
}

type recordingSink struct{ events []recordedEvent }

func (s *recordingSink) Send(name string, data any) error {
	s.events = append(s.events, recordedEvent{name: name, data: data})
	return nil
}

func TestStreamAdapterText(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	adapter := newStreamAdapter("claude-model", sink)
	events := []openai.StreamEvent{
		{Type: "response.created", Response: &openai.Response{ID: "resp_123", Model: "gpt-model"}},
		{Type: "response.output_text.delta", Delta: "hello "},
		{Type: "response.output_text.delta", Delta: "world"},
		{Type: "response.output_text.done"},
		{Type: "response.completed", Response: &openai.Response{ID: "resp_123", Usage: openai.Usage{InputTokens: 7, OutputTokens: 2}}},
	}
	for _, event := range events {
		if err := adapter.Handle(event); err != nil {
			t.Fatal(err)
		}
	}
	result, err := adapter.Result()
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "msg_resp_123" || result.Model != "claude-model" || result.Content[0].Text != "hello world" {
		t.Fatalf("unexpected response: %#v", result)
	}
	if result.Usage.InputTokens != 7 || result.StopReason != "end_turn" {
		t.Fatalf("unexpected usage or stop reason: %#v", result)
	}
	if sink.events[0].name != "message_start" || sink.events[len(sink.events)-1].name != "message_stop" {
		t.Fatalf("unexpected event sequence: %#v", sink.events)
	}
}

func TestStreamAdapterToolUse(t *testing.T) {
	t.Parallel()
	adapter := newStreamAdapter("claude-model", nil)
	item := openai.OutputItem{ID: "item_1", Type: "function_call", CallID: "call_1", Name: "shell"}
	events := []openai.StreamEvent{
		{Type: "response.created", Response: &openai.Response{ID: "r1"}},
		{Type: "response.output_item.added", Item: &item},
		{Type: "response.function_call_arguments.delta", ItemID: "item_1", Delta: `{"cmd":`},
		{Type: "response.function_call_arguments.delta", ItemID: "item_1", Delta: `"pwd"}`},
		{Type: "response.output_item.done", Item: &item},
		{Type: "response.completed", Response: &openai.Response{ID: "r1"}},
	}
	for _, event := range events {
		if err := adapter.Handle(event); err != nil {
			t.Fatal(err)
		}
	}
	result, err := adapter.Result()
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "tool_use" || len(result.Content) != 1 {
		t.Fatalf("unexpected response: %#v", result)
	}
	var input map[string]string
	if err := json.Unmarshal(result.Content[0].Input, &input); err != nil || input["cmd"] != "pwd" {
		t.Fatalf("unexpected tool input: %s (%v)", result.Content[0].Input, err)
	}
}
