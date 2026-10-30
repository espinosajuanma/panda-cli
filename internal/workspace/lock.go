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
