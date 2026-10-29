package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureMigratesLegacyTaskDirectoriesAndLeavesProjectFiles(t *testing.T) {
	root := t.TempDir()
	legacyTask := filepath.Join(root, "23")
	if err := os.MkdirAll(filepath.Join(legacyTask, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteMap(filepath.Join(legacyTask, "metadata.json"), map[string]any{
		"id": "task-23", "number": float64(23), "title": "Legacy task",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyTask, "description.md"), []byte("Task details"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectFiles := filepath.Join(root, "project-notes")
	if err := os.MkdirAll(projectFiles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectFiles, "readme.md"), []byte("Project files stay at the root"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteMap(filepath.Join(projectFiles, "metadata.json"), map[string]any{"id": "project-document"}); err != nil {
		t.Fatal(err)
	}

	ws := New(root)
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	newTaskDir := ws.TaskPath("23")
	if _, err := os.Stat(newTaskDir); err != nil {
		t.Fatalf("migrated task directory missing: %v", err)
	}
	if _, err := os.Stat(legacyTask); !os.IsNotExist(err) {
		t.Fatalf("legacy task directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectFiles, "readme.md")); err != nil {
		t.Fatalf("project file was moved: %v", err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != newTaskDir {
		t.Fatalf("task directories = %v, want [%s]", dirs, newTaskDir)
	}
	if err := ws.Ensure(); err != nil {
		t.Fatalf("second ensure should be idempotent: %v", err)
	}
}

func TestEnsureRejectsLegacyTaskMigrationCollision(t *testing.T) {
	root := t.TempDir()
	legacyTask := filepath.Join(root, "23")
	newTask := filepath.Join(root, "tasks", "23")
	for _, dir := range []string{legacyTask, newTask} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteMap(filepath.Join(legacyTask, "metadata.json"), map[string]any{"id": "task-23", "number": float64(23)}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyTask, "description.md"), []byte("Task details"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New(root).Ensure(); err == nil {
		t.Fatal("expected migration collision error")
	}
	if _, err := os.Stat(filepath.Join(legacyTask, "metadata.json")); err != nil {
		t.Fatalf("legacy task was lost after collision: %v", err)
	}
}

func TestReserveDraftTaskUsesNextNumberAndIgnoresLegacyIDs(t *testing.T) {
	ws := New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	firstID, firstDir, err := ws.ReserveDraftTask()
	if err != nil || firstID != "draft-1" {
		t.Fatalf("first draft = %q, dir=%q, error=%v", firstID, firstDir, err)
	}
	secondID, _, err := ws.ReserveDraftTask()
	if err != nil || secondID != "draft-2" {
		t.Fatalf("second draft = %q, error=%v", secondID, err)
	}
	if err := os.Mkdir(ws.TaskPath("draft-7"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyDir := ws.TaskPath("local-legacy")
	if err := os.Mkdir(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteMap(filepath.Join(legacyDir, "metadata.json"), map[string]any{"localId": "local-legacy"}); err != nil {
		t.Fatal(err)
	}
	nextID, nextDir, err := ws.ReserveDraftTask()
	if err != nil || nextID != "draft-8" || nextDir != ws.TaskPath("draft-8") {
		t.Fatalf("next draft = %q, dir=%q, error=%v; want draft-8", nextID, nextDir, err)
	}
}
