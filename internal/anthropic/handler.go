package anthropic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codex-bridge/codex-bridge/internal/auth"
	"github.com/codex-bridge/codex-bridge/internal/models"
	"github.com/codex-bridge/codex-bridge/internal/openai"
	"github.com/codex-bridge/codex-bridge/internal/telemetry"
)

type Handler struct {
	client           *openai.Client
	models           models.Catalog
	telemetry        *telemetry.Store
	requestBodyLimit int64
	requestTimeout   time.Duration
}

func NewHandler(client *openai.Client, catalog models.Catalog, store *telemetry.Store, requestBodyLimit int64, requestTimeout time.Duration) *Handler {
	return &Handler{client: client, models: catalog, telemetry: store, requestBodyLimit: requestBodyLimit, requestTimeout: requestTimeout}
}

func (h *Handler) Messages(c *gin.Context) {
	started := time.Now()
	var request MessageRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.requestBodyLimit)
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "request body is too large")
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_request_error", "invalid JSON request: "+err.Error())
		return
	}
	cacheKey := c.GetHeader("x-session-id")
	if cacheKey == "" {
		cacheKey = requestID(c)
	}
	upstream, err := ConvertRequest(request, request.Model, cacheKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.requestTimeout)
	defer cancel()
	if request.Stream {
		response, streamErr := h.stream(c, ctx, request.Model, upstream)
		h.record(request.Model, response.Usage, time.Since(started), streamErr == nil)
		return
	}
	adapter := newStreamAdapter(request.Model, nil)
	if err := h.client.Stream(ctx, upstream, adapter.Handle); err != nil {
		h.record(request.Model, Usage{}, time.Since(started), false)
		writeMappedError(c, err)
		return
	}
	response, err := adapter.Result()
	if err != nil {
		h.record(request.Model, Usage{}, time.Since(started), false)
		writeError(c, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	h.record(request.Model, response.Usage, time.Since(started), true)
	c.JSON(http.StatusOK, response)
}

func (h *Handler) stream(c *gin.Context, ctx context.Context, model string, upstream openai.ResponseRequest) (MessageResponse, error) {
	sink := &ginEventSink{context: c}
	adapter := newStreamAdapter(model, sink)
	err := h.client.Stream(ctx, upstream, adapter.Handle)
	if err == nil {
		return adapter.Result()
	}
	if sink.started {
		_ = sink.Send("error", ErrorResponse{Type: "error", Error: ErrorDetail{Type: errorType(err), Message: err.Error()}})
		return MessageResponse{}, err
	}
	writeMappedError(c, err)
	return MessageResponse{}, err
}

func (h *Handler) Models(c *gin.Context) {
	type modelResponse struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		DisplayName string `json:"display_name,omitempty"`
		CreatedAt   string `json:"created_at,omitempty"`
	}
	advertised := h.models.Models()
	data := make([]modelResponse, 0, len(advertised))
	for _, model := range advertised {
		data = append(data, modelResponse{ID: model.ID, Type: "model", DisplayName: model.DisplayName})
	}
	firstID, lastID := "", ""
	if len(data) > 0 {
		firstID = data[0].ID
		lastID = data[len(data)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "has_more": false, "first_id": firstID, "last_id": lastID})
}

func (h *Handler) record(model string, usage Usage, duration time.Duration, success bool) {
	if h.telemetry == nil {
		return
	}
	h.telemetry.Record(telemetry.Event{
		Timestamp: time.Now(), Model: model, InputTokens: usage.InputTokens,
		OutputTokens: usage.OutputTokens, DurationMS: duration.Milliseconds(), Success: success,
	})
}

func (h *Handler) CountTokens(c *gin.Context) {
	var request MessageRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.requestBodyLimit)
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "invalid JSON request: "+err.Error())
		return
	}
	// OpenAI's ChatGPT-plan model catalog does not expose an Anthropic-compatible
	// tokenizer endpoint. This conservative estimate is intended for client-side
	// context budgeting; the authoritative usage is returned after inference.
	data, _ := json.Marshal(struct {
		System   json.RawMessage `json:"system"`
		Messages []Message       `json:"messages"`
		Tools    []Tool          `json:"tools"`
	}{request.System, request.Messages, request.Tools})
	tokens := (len(data) + 3) / 4
	if tokens < 1 {
		tokens = 1
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": tokens})
}

type ginEventSink struct {
	context *gin.Context
	started bool
}

func (s *ginEventSink) Send(event string, data any) error {
	if !s.started {
		s.context.Header("Content-Type", "text/event-stream")
		s.context.Header("Cache-Control", "no-cache")
		s.context.Header("Connection", "keep-alive")
		s.context.Header("X-Accel-Buffering", "no")
		s.context.Status(http.StatusOK)
		s.started = true
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.context.Writer, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	s.context.Writer.Flush()
	return nil
}

func writeMappedError(c *gin.Context, err error) {
	if errors.Is(err, auth.ErrNotLoggedIn) {
		writeError(c, http.StatusUnauthorized, "authentication_error", err.Error())
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(c, http.StatusGatewayTimeout, "timeout_error", "upstream request timed out")
		return
	}
	var apiError *openai.APIError
	if errors.As(err, &apiError) {
		status := apiError.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeError(c, status, errorType(err), apiError.Message)
		return
	}
	writeError(c, http.StatusBadGateway, "api_error", err.Error())
}

func writeError(c *gin.Context, status int, kind, message string) {
	c.JSON(status, ErrorResponse{Type: "error", Error: ErrorDetail{Type: kind, Message: message}})
}

func errorType(err error) string {
	var apiError *openai.APIError
	if errors.As(err, &apiError) {
		switch apiError.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "authentication_error"
		case http.StatusTooManyRequests:
			return "rate_limit_error"
		case http.StatusBadRequest, http.StatusNotFound:
			return "invalid_request_error"
		}
	}
	return "api_error"
}

func requestID(c *gin.Context) string {
	if value, exists := c.Get("request_id"); exists {
		return fmt.Sprint(value)
	}
	buffer := make([]byte, 12)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}
