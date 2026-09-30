package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/codex-bridge/codex-bridge/internal/openai"
)

type EventSink interface {
	Send(event string, data any) error
}

type streamAdapter struct {
	sink         EventSink
	requestModel string
	response     MessageResponse
	started      bool
	textIndex    int
	textOpen     bool
	nextIndex    int
	tools        map[string]*toolState
	hadToolUse   bool
	completed    bool
}

type toolState struct {
	index     int
	itemID    string
	callID    string
	name      string
	arguments string
	open      bool
}

func newStreamAdapter(model string, sink EventSink) *streamAdapter {
	return &streamAdapter{
		sink: sink, requestModel: model, textIndex: -1, tools: make(map[string]*toolState),
		response: MessageResponse{Type: "message", Role: "assistant", Model: model, Content: []ResponseBlock{}, StopReason: "end_turn"},
	}
}

func (a *streamAdapter) Handle(event openai.StreamEvent) error {
	switch event.Type {
	case "response.created", "response.in_progress":
		if event.Response != nil {
			a.response.ID = anthropicMessageID(event.Response.ID)
		}
		return a.start()
	case "response.output_text.delta":
		if err := a.startText(); err != nil {
			return err
		}
		a.response.Content[a.textIndex].Text += event.Delta
		return a.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": a.textIndex, "delta": map[string]any{"type": "text_delta", "text": event.Delta}})
	case "response.output_text.done":
		return a.stopText()
	case "response.output_item.added":
		if event.Item != nil && event.Item.Type == "function_call" {
			return a.startTool(*event.Item)
		}
	case "response.function_call_arguments.delta":
		tool := a.tools[event.ItemID]
		if tool == nil {
			item := openai.OutputItem{ID: event.ItemID, Type: "function_call", CallID: event.ItemID, Name: "tool"}
			if err := a.startTool(item); err != nil {
				return err
			}
			tool = a.tools[event.ItemID]
		}
		tool.arguments += event.Delta
		return a.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": tool.index, "delta": map[string]any{"type": "input_json_delta", "partial_json": event.Delta}})
	case "response.output_item.done":
		if event.Item != nil && event.Item.Type == "function_call" {
			return a.finishTool(*event.Item)
		}
	case "response.incomplete":
		return fmt.Errorf("OpenAI response was incomplete")
	case "response.completed":
		return a.finish(event.Response)
	}
	return nil
}

func (a *streamAdapter) Result() (MessageResponse, error) {
	if !a.completed {
		return MessageResponse{}, fmt.Errorf("response did not complete")
	}
	if a.response.ID == "" {
		a.response.ID = "msg_unknown"
	}
	return a.response, nil
}

func (a *streamAdapter) start() error {
	if a.started {
		return nil
	}
	a.started = true
	if a.response.ID == "" {
		a.response.ID = "msg_pending"
	}
	return a.emit("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": a.response.ID, "type": "message", "role": "assistant", "model": a.response.Model,
			"content": []ResponseBlock{}, "stop_reason": nil, "stop_sequence": nil, "usage": Usage{},
		},
	})
}

func (a *streamAdapter) startText() error {
	if err := a.start(); err != nil {
		return err
	}
	if a.textIndex < 0 {
		a.textIndex = a.nextIndex
		a.nextIndex++
		a.response.Content = append(a.response.Content, ResponseBlock{Type: "text", Text: ""})
	}
	if a.textOpen {
		return nil
	}
	a.textOpen = true
	return a.emit("content_block_start", map[string]any{"type": "content_block_start", "index": a.textIndex, "content_block": ResponseBlock{Type: "text", Text: ""}})
}

func (a *streamAdapter) stopText() error {
	if !a.textOpen {
		return nil
	}
	a.textOpen = false
	index := a.textIndex
	a.textIndex = -1
	return a.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
}

func (a *streamAdapter) startTool(item openai.OutputItem) error {
	if existing := a.tools[item.ID]; existing != nil {
		return nil
	}
	if err := a.start(); err != nil {
		return err
	}
	if err := a.stopText(); err != nil {
		return err
	}
	callID := item.CallID
	if callID == "" {
		callID = item.ID
	}
	tool := &toolState{index: a.nextIndex, itemID: item.ID, callID: callID, name: item.Name, open: true}
	a.nextIndex++
	a.tools[item.ID] = tool
	a.hadToolUse = true
	a.response.Content = append(a.response.Content, ResponseBlock{Type: "tool_use", ID: callID, Name: item.Name, Input: json.RawMessage(`{}`)})
	return a.emit("content_block_start", map[string]any{
		"type": "content_block_start", "index": tool.index,
		"content_block": ResponseBlock{Type: "tool_use", ID: callID, Name: item.Name, Input: json.RawMessage(`{}`)},
	})
}

func (a *streamAdapter) finishTool(item openai.OutputItem) error {
	tool := a.tools[item.ID]
	if tool == nil {
		if err := a.startTool(item); err != nil {
			return err
		}
		tool = a.tools[item.ID]
	}
	if tool.arguments == "" && item.Arguments != "" {
		tool.arguments = item.Arguments
		if err := a.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": tool.index, "delta": map[string]any{"type": "input_json_delta", "partial_json": item.Arguments}}); err != nil {
			return err
		}
	}
	input := json.RawMessage(tool.arguments)
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if !json.Valid(input) {
		return fmt.Errorf("tool %q returned invalid JSON arguments", tool.name)
	}
	a.response.Content[tool.index].Input = input
	if tool.open {
		tool.open = false
		return a.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": tool.index})
	}
	return nil
}

func (a *streamAdapter) finish(response *openai.Response) error {
	if err := a.start(); err != nil {
		return err
	}
	if err := a.stopText(); err != nil {
		return err
	}
	for _, tool := range a.tools {
		if tool.open {
			if err := a.finishTool(openai.OutputItem{ID: tool.itemID, Type: "function_call", CallID: tool.callID, Name: tool.name, Arguments: tool.arguments}); err != nil {
				return err
			}
		}
	}
	if response != nil {
		a.response.ID = anthropicMessageID(response.ID)
		a.response.Usage = Usage{InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens}
	}
	if a.hadToolUse {
		a.response.StopReason = "tool_use"
	} else {
		a.response.StopReason = "end_turn"
	}
	if err := a.emit("message_delta", map[string]any{
		"type": "message_delta", "delta": map[string]any{"stop_reason": a.response.StopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": a.response.Usage.OutputTokens},
	}); err != nil {
		return err
	}
	if err := a.emit("message_stop", map[string]any{"type": "message_stop"}); err != nil {
		return err
	}
	a.completed = true
	return nil
}

func anthropicMessageID(id string) string {
	if id == "" {
		return "msg_unknown"
	}
	if len(id) >= 4 && id[:4] == "msg_" {
		return id
	}
	return "msg_" + id
}

func (a *streamAdapter) emit(event string, data any) error {
	if a.sink == nil {
		return nil
	}
	return a.sink.Send(event, data)
}
