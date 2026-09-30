package openai

import "encoding/json"

type ResponseRequest struct {
	Model             string          `json:"model"`
	Instructions      string          `json:"instructions,omitempty"`
	Input             []InputItem     `json:"input"`
	Tools             []Tool          `json:"tools,omitempty"`
	ToolChoice        any             `json:"tool_choice,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Store             bool            `json:"store"`
	Stream            bool            `json:"stream"`
	PromptCacheKey    string          `json:"prompt_cache_key,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type InputItem map[string]any

type Tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type StreamEvent struct {
	Type         string         `json:"type"`
	Delta        string         `json:"delta,omitempty"`
	ItemID       string         `json:"item_id,omitempty"`
	OutputIndex  int            `json:"output_index,omitempty"`
	ContentIndex int            `json:"content_index,omitempty"`
	Item         *OutputItem    `json:"item,omitempty"`
	Response     *Response      `json:"response,omitempty"`
	Error        *ResponseError `json:"error,omitempty"`
}

type OutputItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Role      string          `json:"role,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

type Response struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Status     string         `json:"status"`
	Output     []OutputItem   `json:"output"`
	Usage      Usage          `json:"usage"`
	Error      *ResponseError `json:"error,omitempty"`
	Incomplete *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
}
