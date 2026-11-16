package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTaskFilesShowsAttachedAndStagedUploads(t *testing.T) {
	view := TaskFilesView{
		Label: "42",
		Title: "Files task",
		Files: []TaskFile{{Name: "spec.pdf", ID: "file-1", Description: "Spec"}},
		Staged: []StagedTaskFile{
			{Name: "mock.png", Path: "/tmp/mock.png", Description: "Mock"},
			{Name: "missing.txt", Path: "/tmp/missing.txt", SourceMissing: true},
		},
	}
	var output bytes.Buffer
	if err := RenderTaskFiles(view, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"Files for task 42 — Files task",
		"[1] spec.pdf (id: file-1) — Spec",
		"Staged uploads:",
		"[staged] mock.png (source: /tmp/mock.png) — Mock",
		"[staged; source missing] missing.txt (source: /tmp/missing.txt)",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("files view missing %q:\n%s", expected, text)
		}
	}
}

func TestRenderTaskFilesShowsNoAttachedFiles(t *testing.T) {
	var output bytes.Buffer
	if err := RenderTaskFiles(TaskFilesView{Label: "1", Title: "Empty"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No attached files.") {
		t.Fatalf("empty files view:\n%s", output.String())
	}
}
