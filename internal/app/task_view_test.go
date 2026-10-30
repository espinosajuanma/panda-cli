package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestTaskViewRendersMetadataDescriptionNotesAndStagedMarker(t *testing.T) {
	remote := map[string]any{
		"id": "view-task", "number": float64(42), "title": "Readable view", "type": "feature",
		"priority": "high", "status": "inProgress", "description": "<p>View body</p>",
		"project":   map[string]any{"id": "project-1", "label": "Alpha"},
		"createdBy": map[string]any{"id": "user-1", "label": "Ada Lovelace"},
		"createdAt": float64(1793275200000),
		"labels":    []any{map[string]any{"id": "label-1", "label": "urgent"}},
		"assignees": []any{map[string]any{"id": "user-2", "label": "Grace Hopper"}},
		"reviewers": []any{map[string]any{"id": "user-3", "label": "Katherine Johnson"}}, "notes": []any{},
		"files": []any{map[string]any{"file": map[string]any{"id": "file-1", "name": "spec.pdf"}, "description": "Specification"}},
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
	state.StagedFiles = []workspace.StagedFile{{Path: "/tmp/design.png", Name: "design.png"}}
	if err := ws.WriteState("view-task", state); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := viewTask(ws, []string{"42"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Readable view", "Status      inProgress", "Created by  Ada Lovelace",
		"Created at  " + formatTaskCreatedAt(remote["createdAt"]), "Grace Hopper", "Katherine Johnson", "urgent", "Local changes pending push",
		"Task link:", "View body", "file-1", "spec.pdf", "design.png (staged upload)", "Staged note body", "[staged]",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("view output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestTaskViewCanOmitDescriptionAndNotes(t *testing.T) {
	ws, _ := makeSyncedTask(t, viewTestTask())
	var output bytes.Buffer
	if err := viewTask(ws, []string{"42", "--no-description", "--no-notes"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"## Description", "## Notes", "View body"} {
		if strings.Contains(output.String(), omitted) {
			t.Errorf("view output included suppressed content %q:\n%s", omitted, output.String())
		}
	}
	if !strings.Contains(output.String(), "## Metadata") || !strings.Contains(output.String(), "## Files") {
		t.Fatalf("view omitted non-suppressed sections:\n%s", output.String())
	}
	for _, expected := range []string{"Labels      None", "Assignees   None", "Reviewers   None", "Sync        Synced"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("metadata table missing %q:\n%s", expected, output.String())
		}
	}
}

func TestTaskViewJSONIncludesTaskMarkdownNotesAndContext(t *testing.T) {
	ws, dir := makeSyncedTask(t, viewTestTask())
	filename := "20261029T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("Note **body**"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("view-task")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{filename}
	state.StagedFiles = []workspace.StagedFile{{Path: "/tmp/staged.png", Name: "staged.png"}}
	if err := ws.WriteState("view-task", state); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := viewTask(ws, []string{"42", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Task        map[string]any `json:"task"`
		Description string         `json:"description"`
		Notes       []struct {
			Filename string `json:"filename"`
			Markdown string `json:"markdown"`
			Staged   bool   `json:"staged"`
		} `json:"notes"`
		Context struct {
			Link           string                 `json:"link"`
			SyncStatus     string                 `json:"syncStatus"`
			PendingChanges bool                   `json:"pendingChanges"`
			LocalDraft     bool                   `json:"localDraft"`
			StagedFiles    []workspace.StagedFile `json:"stagedFiles"`
		} `json:"context"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("invalid JSON output %q: %v", output.String(), err)
	}
	if document.Task["id"] != "view-task" || document.Description != "View body" {
		t.Fatalf("unexpected task/description: %#v / %q", document.Task, document.Description)
	}
	if len(document.Notes) != 1 || document.Notes[0].Filename != filename || document.Notes[0].Markdown != "Note **body**" || !document.Notes[0].Staged {
		t.Fatalf("unexpected JSON notes: %#v", document.Notes)
	}
	if project, _ := document.Task["project"].(map[string]any); project["label"] != "Alpha" || document.Context.Link == "" || document.Context.SyncStatus != "Local changes pending push" || !document.Context.PendingChanges || document.Context.LocalDraft || len(document.Context.StagedFiles) != 1 {
		t.Fatalf("unexpected view context: %#v", document.Context)
	}
}

func TestTaskViewJSONSuppressionFlagsOmitFields(t *testing.T) {
	ws, _ := makeSyncedTask(t, viewTestTask())
	var output bytes.Buffer
	if err := viewTask(ws, []string{"--json", "42", "--no-description", "--no-notes"}, &output); err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["description"]; exists {
		t.Fatalf("suppressed description is present in JSON: %s", output.String())
	}
	if _, exists := document["notes"]; exists {
		t.Fatalf("suppressed notes are present in JSON: %s", output.String())
	}
}

func TestTaskViewShowsLocalDraftContextWithoutRemoteLink(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	draftID, dir, err := ws.ReserveDraftTask()
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{
		"localId": draftID, "title": "Local draft", "project": map[string]any{"id": "project-1", "label": "Alpha"},
		"type": "feature", "priority": "medium", "status": "new",
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteState(draftID, workspace.State{LocalCreated: true}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := viewTask(ws, []string{draftID, "--no-description", "--no-notes"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Sync        Local draft") || strings.Contains(output.String(), "Task link:") {
		t.Fatalf("unexpected local draft view:\n%s", output.String())
	}
}

func TestTaskViewTableWrapsLongValuesAndUsesNoneForEmptyRelationships(t *testing.T) {
	var output bytes.Buffer
	rows := []taskViewRow{{label: "Reviewers", value: "Ada Lovelace Grace Hopper Katherine Johnson"}}
	writeTaskViewTable(&output, &output, rows, 24)
	if strings.Count(output.String(), "\n") < 2 {
		t.Fatalf("long table value was not wrapped:\n%s", output.String())
	}
	if got := displayRelationship(map[string]any{}); got != "" {
		t.Fatalf("empty relationship displayed as %q", got)
	}
}

func viewTestTask() map[string]any {
	return map[string]any{
		"id": "view-task", "number": float64(42), "title": "Readable view", "type": "feature",
		"priority": "high", "status": "inProgress", "description": "<p>View body</p>",
		"project":   map[string]any{"id": "project-1", "label": "Alpha"},
		"createdBy": map[string]any{"id": "user-1", "label": "Ada Lovelace"}, "createdAt": float64(1793275200000),
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "files": []any{}, "notes": []any{},
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
