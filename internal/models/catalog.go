package models

import "fmt"

type Catalog struct {
	Haiku  string
	Sonnet string
	Opus   string
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Tier        string `json:"tier"`
}

func NewCatalog(haiku, sonnet, opus string) Catalog {
	return Catalog{Haiku: haiku, Sonnet: sonnet, Opus: opus}
}

func (c Catalog) Models() []Model {
	candidates := []Model{
		{ID: c.Haiku, DisplayName: "Codex Haiku", Tier: "haiku"},
		{ID: c.Sonnet, DisplayName: "Codex Sonnet", Tier: "sonnet"},
		{ID: c.Opus, DisplayName: "Codex Opus", Tier: "opus"},
	}
	seen := make(map[string]bool, len(candidates))
	result := make([]Model, 0, len(candidates))
	for _, model := range candidates {
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		result = append(result, model)
	}
	return result
}

func (c Catalog) String() string {
	return fmt.Sprintf("haiku=%s sonnet=%s opus=%s", c.Haiku, c.Sonnet, c.Opus)
}
