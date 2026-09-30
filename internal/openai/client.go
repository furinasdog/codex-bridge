package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type TokenProvider interface {
	Token(context.Context) (string, error)
}

type Client struct {
	baseURL   string
	tokens    TokenProvider
	http      *http.Client
	userAgent string
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("OpenAI API error (%s): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("OpenAI API error (HTTP %d): %s", e.StatusCode, e.Message)
}

func NewClient(baseURL string, tokens TokenProvider, httpClient *http.Client, userAgent string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 0}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), tokens: tokens, http: httpClient, userAgent: userAgent}
}

func (c *Client) Stream(ctx context.Context, input ResponseRequest, handle func(StreamEvent) error) error {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode Responses request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call OpenAI Responses API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return decodeAPIError(response)
	}
	return consumeSSE(response.Body, handle)
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list OpenAI models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, decodeAPIError(response)
	}
	var body struct {
		Models []Model `json:"models"`
		Data   []Model `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode OpenAI model list: %w", err)
	}
	if len(body.Models) > 0 {
		return body.Models, nil
	}
	return body.Data, nil
}

func consumeSSE(reader io.Reader, handle func(StreamEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	var data strings.Builder
	completed := false
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := data.String()
		data.Reset()
		if raw == "[DONE]" {
			return nil
		}
		var event StreamEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return fmt.Errorf("decode Responses stream event: %w", err)
		}
		if event.Type == "response.completed" {
			completed = true
		}
		if event.Type == "response.failed" || event.Type == "error" {
			apiErr := &APIError{Message: "response failed"}
			if event.Error != nil {
				apiErr.Code, apiErr.Message = event.Error.Code, event.Error.Message
			} else if event.Response != nil && event.Response.Error != nil {
				apiErr.Code, apiErr.Message = event.Response.Error.Code, event.Response.Error.Message
			}
			return apiErr
		}
		return handle(event)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Responses stream: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}
	if !completed {
		return errors.New("Responses stream ended before response.completed")
	}
	return nil
}

func decodeAPIError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var envelope struct {
		Error ResponseError `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	code := ""
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		message = envelope.Error.Message
		code = envelope.Error.Code
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return &APIError{StatusCode: response.StatusCode, Code: code, Message: message}
}
