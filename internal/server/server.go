package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codex-bridge/codex-bridge/internal/anthropic"
	"github.com/codex-bridge/codex-bridge/internal/config"
	"github.com/codex-bridge/codex-bridge/internal/openai"
)

type Server struct {
	cfg    config.Config
	server *http.Server
}

func New(cfg config.Config, tokens openai.TokenProvider, version string) *Server {
	client := openai.NewClient(cfg.UpstreamURL, tokens, nil, cfg.ForwardUserAgent+"/"+version)
	handler := anthropic.NewHandler(client, cfg.UpstreamModel, cfg.RequestBodyLimit, cfg.RequestTimeout)
	router := NewRouter(cfg, handler)
	return &Server{
		cfg: cfg,
		server: &http.Server{
			Addr: cfg.Address, Handler: router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       90 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}
}

func NewRouter(cfg config.Config, handler *anthropic.Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.Recovery(), securityHeaders(), requestLogger(), cors(cfg.AllowedOrigins))
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	v1 := router.Group("/v1")
	v1.Use(apiKey(cfg.APIKey))
	v1.POST("/messages", handler.Messages)
	v1.POST("/messages/count_tokens", handler.CountTokens)
	v1.GET("/models", handler.Models)
	return router
}

func (s *Server) Run(ctx context.Context) error {
	if !isLoopbackAddress(s.cfg.Address) && s.cfg.APIKey == "" {
		return fmt.Errorf("refusing to listen on a non-loopback address without CODEX_BRIDGE_API_KEY")
	}
	errorChannel := make(chan error, 1)
	go func() {
		slog.Info("server started", "address", s.cfg.Address, "model", s.cfg.UpstreamModel)
		errorChannel <- s.server.ListenAndServe()
	}()
	select {
	case err := <-errorChannel:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" || len(requestID) > 128 {
			buffer := make([]byte, 12)
			_, _ = rand.Read(buffer)
			requestID = hex.EncodeToString(buffer)
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		started := time.Now()
		c.Next()
		slog.Info("request completed", "request_id", requestID, "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

func apiKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expected == "" {
			c.Next()
			return
		}
		provided := c.GetHeader("x-api-key")
		if provided == "" {
			provided = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, anthropic.ErrorResponse{Type: "error", Error: anthropic.ErrorDetail{Type: "authentication_error", Message: "invalid bridge API key"}})
			return
		}
		c.Next()
	}
}

func cors(allowed []string) gin.HandlerFunc {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		allowedSet[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowedSet[origin]; ok && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, anthropic-version, anthropic-beta")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if origin != "" {
				if _, ok := allowedSet[origin]; !ok {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
