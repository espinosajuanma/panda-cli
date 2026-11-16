package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s; run `UPDATE_GOLDEN=1 go test ./internal/app -run Golden`\n--- got ---\n%s", name, got)
	}
}

func TestGoldenHelp(t *testing.T) {
	var output bytes.Buffer
	if err := printHelp(&output); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "help.txt", output.Bytes())
}

func TestGoldenHelpPush(t *testing.T) {
	var output bytes.Buffer
	if err := printCommandHelp("push", &output); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "help-push.txt", output.Bytes())
}

func TestGoldenListJSON(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("golden-todo", 12, "toDo", "b", "m", "Golden todo", "feature"),
		listTask("golden-progress", 11, "inProgress", "a", "s", "Golden progress", "bug"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}
	var output bytes.Buffer
	if err := listTasksForUser(ws, []string{"--json"}, "", &output); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "list.json", output.Bytes())
}

func TestGoldenDiffJSON(t *testing.T) {
	remote := map[string]any{
		"id": "golden-task", "number": float64(7), "title": "Golden task", "type": "feature",
		"priority": "medium", "status": "toDo", "description": "<p>Original</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{},
		"assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	ws, dir := makeSyncedTask(t, remote)
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["priority"] = "high"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte("Updated"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := diff(ws, []string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "diff.json", output.Bytes())
}
