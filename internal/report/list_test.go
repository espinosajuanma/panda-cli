package report

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func testListTask(id string, number float64, status, rank string, complexity any, title, taskType string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "title": title, "type": taskType,
		"priority": "medium", "complexity": complexity, "status": status, "rank": rank,
		"description": "", "project": map[string]any{"id": "project-1"},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
}

func TestResponsiveListDropsOptionalColumnsAndWrapsTitles(t *testing.T) {
	task := testListTask("responsive", 61, "new", "a", "m", "Implement a responsive terminal layout for wide task tables", "feature")
	task["package"] = map[string]any{"id": "package-1", "label": "Long package name"}
	task["labels"] = []any{map[string]any{"id": "label-1", "label": "long-label-name"}}
	task["assignees"] = []any{map[string]any{"id": "user-1", "label": "Ada Lovelace"}}
	task["reviewers"] = []any{map[string]any{"id": "user-2", "label": "Grace Hopper"}}

	var output strings.Builder
	if _, err := PrintTaskGroup(&output, &output, "new", []map[string]any{task}, nil, nil, true, true, 60); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, hidden := range []string{"PACKAGE", "LABELS", "ASSIGNEES", "REVIEWERS", "HOURS"} {
		if strings.Contains(text, hidden) {
			t.Errorf("narrow table retained lower-priority column %q:\n%s", hidden, text)
		}
	}
	for _, shown := range []string{"NUMBER", "PRIORITY", "TYPE", "COMPLEXITY", "SP", "TITLE"} {
		if !strings.Contains(text, shown) {
			t.Errorf("narrow table hid expected column %q:\n%s", shown, text)
		}
	}
	if !strings.Contains(text, "Implement") || !strings.Contains(text, "tables") {
		t.Fatalf("wrapped title content was lost:\n%s", text)
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if width := utf8.RuneCountInString(line); width > 60 {
			t.Errorf("responsive output line has %d columns, want at most 60: %q", width, line)
		}
	}
}
