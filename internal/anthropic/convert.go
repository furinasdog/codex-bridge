package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codex-bridge/codex-bridge/internal/openai"
)

func ConvertRequest(input MessageRequest, upstreamModel, cacheKey string) (openai.ResponseRequest, error) {
	if input.MaxTokens <= 0 {
		return openai.ResponseRequest{}, fmt.Errorf("max_tokens must be greater than zero")
	}
	if len(input.Messages) == 0 {
		return openai.ResponseRequest{}, fmt.Errorf("messages must not be empty")
	}
	instructions, err := parseSystem(input.System)
	if err != nil {
		return openai.ResponseRequest{}, err
	}
	items := make([]openai.InputItem, 0, len(input.Messages))
	for index, message := range input.Messages {
		converted, err := convertMessage(message)
		if err != nil {
			return openai.ResponseRequest{}, fmt.Errorf("messages[%d]: %w", index, err)
		}
		items = append(items, converted...)
	}
	tools := make([]openai.Tool, 0, len(input.Tools))
	for index, tool := range input.Tools {
		if tool.Name == "" || len(tool.InputSchema) == 0 || !json.Valid(tool.InputSchema) {
			return openai.ResponseRequest{}, fmt.Errorf("tools[%d] has an invalid name or input_schema", index)
		}
		tools = append(tools, openai.Tool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema, Strict: false})
	}
	toolChoice, parallel, err := convertToolChoice(input.ToolChoice)
	if err != nil {
		return openai.ResponseRequest{}, err
	}
	if len(cacheKey) > 64 {
		cacheKey = cacheKey[:64]
	}
	return openai.ResponseRequest{
		Model: upstreamModel, Instructions: instructions, Input: items, Tools: tools,
		ToolChoice: toolChoice, MaxOutputTokens: input.MaxTokens, Temperature: input.Temperature,
		TopP: input.TopP, ParallelToolCalls: parallel, Store: false, Stream: true,
		PromptCacheKey: cacheKey,
	}, nil
}

func parseSystem(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("system must be a string or an array of text blocks")
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != "text" {
			return "", fmt.Errorf("system block type %q is not supported", block.Type)
		}
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n\n"), nil
}

func convertMessage(message Message) ([]openai.InputItem, error) {
	if message.Role != "user" && message.Role != "assistant" {
		return nil, fmt.Errorf("role must be user or assistant")
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		return []openai.InputItem{{"role": message.Role, "content": text}}, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(message.Content, &blocks); err != nil {
		return nil, fmt.Errorf("content must be a string or an array of content blocks")
	}
	result := make([]openai.InputItem, 0, len(blocks))
	var messageContent []map[string]any
	flushContent := func() {
		if len(messageContent) > 0 {
			result = append(result, openai.InputItem{"role": message.Role, "content": messageContent})
			messageContent = nil
		}
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			contentType := "input_text"
			if message.Role == "assistant" {
				contentType = "output_text"
			}
			messageContent = append(messageContent, map[string]any{"type": contentType, "text": block.Text})
		case "image":
			if message.Role != "user" || block.Source == nil {
				return nil, fmt.Errorf("image blocks are only supported in user messages")
			}
			imageURL := block.Source.URL
			if block.Source.Type == "base64" {
				imageURL = "data:" + block.Source.MediaType + ";base64," + block.Source.Data
			}
			if imageURL == "" {
				return nil, fmt.Errorf("image source is empty")
			}
			messageContent = append(messageContent, map[string]any{"type": "input_image", "image_url": imageURL})
		case "tool_use":
			if message.Role != "assistant" || block.ID == "" || block.Name == "" || !json.Valid(block.Input) {
				return nil, fmt.Errorf("invalid assistant tool_use block")
			}
			flushContent()
			result = append(result, openai.InputItem{"type": "function_call", "call_id": block.ID, "name": block.Name, "arguments": string(block.Input)})
		case "tool_result":
			if message.Role != "user" || block.ToolUseID == "" {
				return nil, fmt.Errorf("invalid user tool_result block")
			}
			flushContent()
			output, err := toolResultText(block.Content)
			if err != nil {
				return nil, err
			}
			result = append(result, openai.InputItem{"type": "function_call_output", "call_id": block.ToolUseID, "output": output})
		default:
			return nil, fmt.Errorf("content block type %q is not supported", block.Type)
		}
	}
	flushContent()
	return result, nil
}

func toolResultText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Type != "text" {
				return "", fmt.Errorf("tool_result supports only text content")
			}
			parts = append(parts, block.Text)
		}
		return strings.Join(parts, "\n"), nil
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	return "", fmt.Errorf("tool_result content must be a string or text blocks")
}

func convertToolChoice(choice *ToolChoice) (any, *bool, error) {
	if choice == nil {
		return nil, nil, nil
	}
	parallel := !choice.DisableParallelToolUse
	switch choice.Type {
	case "auto":
		return "auto", &parallel, nil
	case "any":
		return "required", &parallel, nil
	case "none":
		return "none", &parallel, nil
	case "tool":
		if choice.Name == "" {
			return nil, nil, fmt.Errorf("tool_choice.name is required for type tool")
		}
		return map[string]string{"type": "function", "name": choice.Name}, &parallel, nil
	default:
		return nil, nil, fmt.Errorf("unsupported tool_choice type %q", choice.Type)
	}
}
