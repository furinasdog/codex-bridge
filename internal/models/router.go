package models

import (
	"fmt"
	"strings"
)

const (
	HaikuAlias  = "codex-haiku"
	SonnetAlias = "codex-sonnet"
	OpusAlias   = "codex-opus"
)

type Router struct {
	Haiku  string
	Sonnet string
	Opus   string
}

type Advertised struct {
	ID          string
	DisplayName string
	Target      string
}

func NewRouter(haiku, sonnet, opus string) Router {
	return Router{Haiku: haiku, Sonnet: sonnet, Opus: opus}
}

func (r Router) Resolve(requested string) string {
	value := strings.ToLower(strings.TrimSpace(requested))
	switch {
	case requested == r.Haiku || requested == r.Sonnet || requested == r.Opus:
		return requested
	case value == HaikuAlias || strings.Contains(value, "haiku") || strings.Contains(value, "luna"):
		return r.Haiku
	case value == OpusAlias || strings.Contains(value, "opus") || strings.Contains(value, "fable") || strings.Contains(value, "astra") || value == "best":
		return r.Opus
	case value == SonnetAlias || strings.Contains(value, "sonnet") || strings.Contains(value, "sol") || value == "default" || value == "":
		return r.Sonnet
	default:
		// Preserve explicitly selected gateway-discovered model IDs. OpenAI
		// remains the authority on whether the signed-in account may use them.
		return requested
	}
}

func (r Router) AdvertisedModels() []Advertised {
	return []Advertised{
		{ID: HaikuAlias, DisplayName: fmt.Sprintf("Codex Haiku (%s)", r.Haiku), Target: r.Haiku},
		{ID: SonnetAlias, DisplayName: fmt.Sprintf("Codex Sonnet (%s)", r.Sonnet), Target: r.Sonnet},
		{ID: OpusAlias, DisplayName: fmt.Sprintf("Codex Opus (%s)", r.Opus), Target: r.Opus},
	}
}

func (r Router) String() string {
	return fmt.Sprintf("haiku=%s sonnet=%s opus=%s", r.Haiku, r.Sonnet, r.Opus)
}
