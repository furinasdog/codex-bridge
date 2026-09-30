package models

import (
	"reflect"
	"testing"

	"github.com/codex-bridge/codex-bridge/internal/openai"
)

func TestResolve(t *testing.T) {
	t.Parallel()
	router := NewRouter("fast", "balanced", "powerful")
	tests := map[string]string{
		"":                         "balanced",
		"codex-haiku":              "fast",
		"claude-haiku-4-5":         "fast",
		"codex-sonnet":             "balanced",
		"claude-sonnet-4-5":        "balanced",
		"codex-opus":               "powerful",
		"claude-opus-4-6":          "powerful",
		"account-specific-preview": "account-specific-preview",
	}
	for requested, expected := range tests {
		requested, expected := requested, expected
		t.Run(requested, func(t *testing.T) {
			t.Parallel()
			if actual := router.Resolve(requested); actual != expected {
				t.Fatalf("Resolve(%q) = %q, want %q", requested, actual, expected)
			}
		})
	}
}

func TestAdvertisedModelsOnlyIncludesAvailableTargets(t *testing.T) {
	t.Parallel()
	router := NewRouter("fast", "balanced", "powerful")
	catalog := []openai.Model{
		{Slug: "fast", Visibility: "list"},
		{ID: "balanced"},
		{Slug: "powerful", Visibility: "hidden"},
	}
	actual := router.AdvertisedModels(catalog)
	want := []Advertised{
		{ID: HaikuAlias, DisplayName: "Codex Haiku (fast)", Target: "fast"},
		{ID: SonnetAlias, DisplayName: "Codex Sonnet (balanced)", Target: "balanced"},
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("AdvertisedModels() = %#v, want %#v", actual, want)
	}
}
