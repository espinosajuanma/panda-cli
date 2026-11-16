package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestTaskViewTableWrapsLongValues(t *testing.T) {
	var output bytes.Buffer
	rows := []TaskViewRow{{Label: "Reviewers", Value: "Ada Lovelace Grace Hopper Katherine Johnson"}}
	WriteTaskViewTable(&output, &output, rows, 24)
	if strings.Count(output.String(), "\n") < 2 {
		t.Fatalf("long table value was not wrapped:\n%s", output.String())
	}
}

func TestRenderTaskViewTextOmitsHiddenSections(t *testing.T) {
	view := TaskViewText{
		Title:     "42 — Test",
		Rows:      []TaskViewRow{{Label: "Status", Value: "IN PROGRESS"}},
		Files:     []TaskViewFile{{Name: "spec.pdf", ID: "file-1"}},
		Notes:     []TaskViewNote{{Filename: "note.md", Markdown: "body", Staged: true}},
		ShowNotes: true,
	}
	var output bytes.Buffer
	if err := RenderTaskViewText(view, &output, &output, 80); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"# 42 — Test", "## Metadata", "## Files", "spec.pdf (id: file-1)", "## Notes", "### note [staged]"} {
		if !strings.Contains(text, expected) {
			t.Errorf("task view missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "## Description") || strings.Contains(text, "## customer notes") {
		t.Fatalf("hidden task view sections were rendered:\n%s", text)
	}
}
