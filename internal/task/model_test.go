package task

import "testing"

func TestFromMapPreservesUnknownFields(t *testing.T) {
	values := map[string]any{"id": "task-1", "title": "Title", "custom": map[string]any{"nested": true}}

	task := FromMap(values)
	task.Set("title", "Changed")
	task.Set("extra", float64(1))
	task.Delete("id")

	if values["id"] != nil {
		t.Fatal("Delete did not affect the underlying map")
	}
	if values["title"] != "Changed" || values["extra"] != float64(1) {
		t.Fatalf("mutations not visible in underlying map: %#v", values)
	}
	if _, ok := values["custom"]; !ok {
		t.Fatalf("unknown field was dropped: %#v", values)
	}
}

func TestCloneIsDeepAndIndependent(t *testing.T) {
	original := FromMap(map[string]any{"id": "task-1", "labels": []any{map[string]any{"id": "label-1"}}})
	clone := original.Clone()
	clone.Set("id", "task-2")
	clone.Field("labels").([]any)[0].(map[string]any)["id"] = "label-2"

	if original.StringField("id") != "task-1" {
		t.Fatalf("clone mutation changed original id: %q", original.StringField("id"))
	}
	if got := original.Field("labels").([]any)[0].(map[string]any)["id"]; got != "label-1" {
		t.Fatalf("clone mutation changed nested original value: %v", got)
	}
}
