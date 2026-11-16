package workspace

import (
	"errors"
	"os"
	"path/filepath"
)

func ReadTextFile(dir, name string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func WriteTextFile(dir, name, contents string) error {
	return WriteFileAtomic(filepath.Join(dir, name), []byte(contents), 0o600)
}

func EnsureTextFile(dir, name, contents string) error {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return WriteTextFile(dir, name, contents)
}
