package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestTaskViewRendersMetadataDescriptionNotesAndStagedMarker(t *testing.T) {
	remote := map[string]any{
		"id": "view-task", "number": float64(42), "title": "Readable view", "type": "feature",
		"priority": "high", "status": "inProgress", "description": "<p>View body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	ws, dir := makeSyncedTask(t, remote)
	filename := "20261029T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("Staged note body"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("view-task")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{filename}
	if err := ws.WriteState("view-task", state); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := viewTask(ws, []string{"42"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Readable view", "**status:** inProgress", "View body", "Staged note body", "[staged]"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("view output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestPagedOutputDecisionUsesTerminalDimensions(t *testing.T) {
	short := []byte("short view\n")
	long := []byte(strings.Repeat("view line\n", 30))
	if shouldPageOutput(false, long, 80, 24) {
		t.Fatal("non-terminal output should not be paged")
	}
	if shouldPageOutput(true, short, 80, 24) {
		t.Fatal("output that fits the terminal should not be paged")
	}
	if !shouldPageOutput(true, long, 80, 24) {
		t.Fatal("output longer than the terminal should be paged")
	}
}
