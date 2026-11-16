package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (w *Workspace) ReadMembers() ([]map[string]any, bool, error) {
	path := filepath.Join(w.Root, ".panda", "members.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var members []map[string]any
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, false, fmt.Errorf("parse project members: %w", err)
	}
	return members, true, nil
}

func (w *Workspace) WriteMembers(members []map[string]any) error {
	data, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(w.Root, ".panda", "members.json"), append(data, '\n'), 0o600)
}
