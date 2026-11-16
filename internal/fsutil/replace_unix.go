//go:build !windows

package fsutil

import "os"

func Replace(source, destination string) error { return os.Rename(source, destination) }
