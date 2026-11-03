package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

type Lock struct {
	file *os.File
}

// LockWorkspace obtains an exclusive process lock for mutations to a workspace.
func (w *Workspace) LockWorkspace() (*Lock, error) {
	path := filepath.Join(w.Root, ".panda", "workspace.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect workspace lock: %w", err)
	}
	if err := acquireFileLock(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("workspace is in use by another panda process: %w", err)
	}
	return &Lock{file: file}, nil
}

// ProbeLock reports whether a lock file exists and whether another process
// currently holds it. It never creates the lock file.
func (w *Workspace) ProbeLock() (exists, held bool, err error) {
	path := filepath.Join(w.Root, ".panda", "workspace.lock")
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("open workspace lock for inspection: %w", err)
	}
	defer file.Close()
	if err := acquireFileLock(file); err != nil {
		if isFileLockHeld(err) {
			return true, true, nil
		}
		return true, false, fmt.Errorf("probe workspace lock: %w", err)
	}
	if err := releaseFileLock(file); err != nil {
		return true, false, fmt.Errorf("release inspected workspace lock: %w", err)
	}
	return true, false, nil
}

func (lock *Lock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := releaseFileLock(lock.file)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
