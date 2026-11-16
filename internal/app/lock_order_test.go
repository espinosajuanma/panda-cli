package app

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func saveTestProject(t *testing.T) *config.Config {
	t.Helper()
	setTestConfigHome(t)
	t.Setenv("PANDA_API_URL", "")
	cfg := &config.Config{
		APIURL:  config.DefaultAPIURL,
		Project: &config.Project{ID: "project-lock", Name: "Lock Project"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestMutatingCommandLocksBeforeMigration(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("workspace locks are unsupported on this platform")
	}
	cfg := saveTestProject(t)
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(root, "23")
	if err := os.MkdirAll(filepath.Join(legacyDir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteMap(filepath.Join(legacyDir, "metadata.json"), map[string]any{
		"id": "task-23", "number": float64(23), "title": "Legacy task",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "description.md"), []byte("Legacy"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(root)
	lock, err := ws.LockWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	err = Run([]string{"create", "--title", "Locked"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "workspace is in use") {
		t.Fatalf("mutating command error = %v, want workspace in use", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "tasks", "23")); !os.IsNotExist(statErr) {
		t.Fatalf("legacy task was migrated before the lock was acquired: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(legacyDir, "metadata.json")); statErr != nil {
		t.Fatalf("legacy task was lost: %v", statErr)
	}
}

func TestReadOnlyCommandDoesNotInitializeMissingWorkspace(t *testing.T) {
	cfg := saveTestProject(t)
	err := Run([]string{"list"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "run `panda pull` first") {
		t.Fatalf("read-only command error = %v, want missing workspace guidance", err)
	}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("read-only command created the workspace: %v", statErr)
	}
}

func TestReadOnlyCommandDoesNotMigrateLegacyWorkspace(t *testing.T) {
	cfg := saveTestProject(t)
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(root, "23")
	if err := os.MkdirAll(filepath.Join(legacyDir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteMap(filepath.Join(legacyDir, "metadata.json"), map[string]any{
		"id": "task-23", "number": float64(23), "title": "Legacy task", "status": "new",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "description.md"), []byte("Legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"list"}, strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "tasks", "23")); !os.IsNotExist(statErr) {
		t.Fatalf("read-only command migrated the legacy task: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(legacyDir, "metadata.json")); statErr != nil {
		t.Fatalf("legacy task was lost: %v", statErr)
	}
}
