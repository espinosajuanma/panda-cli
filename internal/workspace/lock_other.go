//go:build !unix && !windows

package workspace

import (
	"errors"
	"os"
)

func acquireFileLock(file *os.File) error {
	return errors.New("workspace locks are unsupported on this platform")
}
func isFileLockHeld(error) bool           { return false }
func releaseFileLock(file *os.File) error { return nil }
