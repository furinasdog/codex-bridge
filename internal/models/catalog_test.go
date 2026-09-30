package models

import "testing"

func TestCatalogAdvertisesConfiguredModels(t *testing.T) {
	t.Parallel()
	catalog := NewCatalog("fast", "balanced", "powerful")
	models := catalog.Models()
	want := []string{"fast", "balanced", "powerful"}
	if len(models) != len(want) {
		t.Fatalf("Models() length = %d, want %d", len(models), len(want))
	}
	for index, id := range want {
		if models[index].ID != id {
			t.Fatalf("Models()[%d].ID = %q, want %q", index, models[index].ID, id)
		}
	}
}

func TestCatalogDeduplicatesSharedModel(t *testing.T) {
	t.Parallel()
	models := NewCatalog("same", "same", "same").Models()
	if len(models) != 1 || models[0].ID != "same" {
		t.Fatalf("Models() = %#v, want one shared model", models)
	}
}
