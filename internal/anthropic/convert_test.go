package anthropic

import (
	"encoding/json"
	"testing"
)

func TestConvertRequestWithToolsAndImages(t *testing.T) {
	t.Parallel()
	request := MessageRequest{
		Model:     "claude-sonnet",
		MaxTokens: 1024,
		System:    json.RawMessage(`[{"type":"text","text":"Be concise."}]`),
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"Inspect this"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"read_file","input":{"path":"a.go"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"package main"}]`)},
		},
		Tools:      []Tool{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: &ToolChoice{Type: "auto", DisableParallelToolUse: true},
	}
	converted, err := ConvertRequest(request, "gpt-test", "a-cache-key")
	if err != nil {
		t.Fatalf("ConvertRequest() error = %v", err)
	}
	if converted.Model != "gpt-test" || converted.Instructions != "Be concise." {
		t.Fatalf("unexpected model or instructions: %#v", converted)
	}
	if !converted.Stream || converted.Store {
		t.Fatalf("request must use stream=true and store=false")
	}
	if len(converted.Input) != 3 {
		t.Fatalf("got %d input items, want 3", len(converted.Input))
	}
	if converted.Input[1]["type"] != "function_call" || converted.Input[2]["type"] != "function_call_output" {
		t.Fatalf("tool items were not converted correctly: %#v", converted.Input)
	}
	if converted.ToolChoice != "auto" || converted.ParallelToolCalls == nil || *converted.ParallelToolCalls {
		t.Fatalf("tool choice was not converted correctly")
	}
}

func TestConvertRequestRejectsInvalidContent(t *testing.T) {
	t.Parallel()
	request := MessageRequest{MaxTokens: 1, Messages: []Message{{Role: "user", Content: json.RawMessage(`[{"type":"document"}]`)}}}
	if _, err := ConvertRequest(request, "model", "key"); err == nil {
		t.Fatal("ConvertRequest() succeeded for unsupported content")
	}
}

func TestConvertRequestClampsCacheKey(t *testing.T) {
	t.Parallel()
	request := MessageRequest{MaxTokens: 1, Messages: []Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
	converted, err := ConvertRequest(request, "model", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-extra")
	if err != nil {
		t.Fatal(err)
	}
	if len(converted.PromptCacheKey) != 64 {
		t.Fatalf("cache key length = %d, want 64", len(converted.PromptCacheKey))
	}
}

func TestConvertRequestMergesMessageLevelInstructions(t *testing.T) {
	t.Parallel()
	request := MessageRequest{
		MaxTokens: 32,
		System:    json.RawMessage(`"top-level"`),
		Messages: []Message{
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"Claude harness"}]`)},
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "developer", Content: json.RawMessage(`"gateway policy"`)},
		},
	}
	converted, err := ConvertRequest(request, "model", "key")
	if err != nil {
		t.Fatal(err)
	}
	if converted.Instructions != "top-level\n\nClaude harness\n\ngateway policy" {
		t.Fatalf("instructions = %q", converted.Instructions)
	}
	if len(converted.Input) != 1 || converted.Input[0]["role"] != "user" {
		t.Fatalf("unexpected input: %#v", converted.Input)
	}
}
