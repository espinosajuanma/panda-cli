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
