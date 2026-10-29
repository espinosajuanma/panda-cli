package jsonutil

import "testing"

func TestCloneMapCopiesNestedJSONValues(t *testing.T) {
	original := map[string]any{"nested": map[string]any{"title": "before"}}
	clone := CloneMap(original)
	clone["nested"].(map[string]any)["title"] = "after"
	if got := original["nested"].(map[string]any)["title"]; got != "before" {
		t.Fatalf("mutating clone changed original: %v", got)
	}
}

func TestValueAtAndStringFieldTreatMissingValuesAsEmpty(t *testing.T) {
	values := map[string]any{"name": "Panda"}
	if ValueAt(values, "missing") != nil {
		t.Fatal("missing value should be nil")
	}
	if got := StringField(values, "missing"); got != "" {
		t.Fatalf("missing string field = %q, want empty", got)
	}
}
