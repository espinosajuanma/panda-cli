package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockWorkspaceCreatesPandaDirectoryWithoutInitializingTasks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	ws := New(root)
	lock, err := ws.LockWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	info, err := os.Stat(filepath.Join(root, ".panda"))
	if err != nil {
		t.Fatalf("lock directory missing: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("lock directory mode = %o, want 700", info.Mode().Perm())
	}
	if _, err := os.Stat(ws.TasksRoot()); !os.IsNotExist(err) {
		t.Fatalf("lock created tasks directory: %v", err)
	}
}
