//go:build !windows

package workspace

import "os"

func ReplaceFile(source, destination string) error { return os.Rename(source, destination) }
