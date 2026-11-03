package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStateKeysListsDecodedKeysWithoutMutation(t *testing.T) {
	ws := New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"task-2", "catalog:labels", "task-1"} {
		if err := ws.WriteState(key, State{}); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := ws.StateKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, []string{"catalog:labels", "task-1", "task-2"}) {
		t.Fatalf("state keys = %v", keys)
	}
}

func TestProbeLockDoesNotCreateAndDetectsHeldLock(t *testing.T) {
	ws := New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(ws.Root, ".panda", "workspace.lock")
	exists, held, err := ws.ProbeLock()
	if err != nil || exists || held {
		t.Fatalf("probe absent lock = exists %v held %v err %v", exists, held, err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("read-only probe created the lock file: %v", err)
	}
	lock, err := ws.LockWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	exists, held, err = ws.ProbeLock()
	if err != nil || !exists || !held {
		t.Fatalf("probe held lock = exists %v held %v err %v", exists, held, err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	exists, held, err = ws.ProbeLock()
	if err != nil || !exists || held {
		t.Fatalf("probe released lock = exists %v held %v err %v", exists, held, err)
	}
}
