package workspace

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Workspace struct {
	Root string
}

type State struct {
	Remote       map[string]any `json:"remote,omitempty"`
	StagedNotes  []string       `json:"staged_notes,omitempty"`
	LocalCreated bool           `json:"local_created,omitempty"`
}

func New(root string) *Workspace {
	return &Workspace{Root: root}
}

func (w *Workspace) Ensure() error {
	for _, dir := range []string{w.Root, filepath.Join(w.Root, ".panda"), filepath.Join(w.Root, ".panda", "state"), w.TasksRoot()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return w.migrateLegacyTaskDirectories()
}

func (w *Workspace) TasksRoot() string {
	return filepath.Join(w.Root, "tasks")
}

func (w *Workspace) TaskPath(name string) string {
	return filepath.Join(w.TasksRoot(), name)
}

func (w *Workspace) CatalogPath(name string) string {
	return filepath.Join(w.Root, ".panda", name+".json")
}

func (w *Workspace) CatalogStateKey(name string) string {
	return "catalog:" + name
}

func (w *Workspace) migrateLegacyTaskDirectories() error {
	entries, err := os.ReadDir(w.Root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".panda" || entry.Name() == "tasks" {
			continue
		}
		legacyDir := filepath.Join(w.Root, entry.Name())
		metadata, err := ReadMap(filepath.Join(legacyDir, "metadata.json"))
		if err != nil || (metadata["id"] == nil && metadata["localId"] == nil && metadata["number"] == nil) {
			continue
		}
		if !hasTaskFiles(legacyDir) {
			continue
		}
		newDir := w.TaskPath(entry.Name())
		if _, err := os.Lstat(newDir); err == nil {
			return fmt.Errorf("cannot migrate legacy task directory %s: %s already exists", entry.Name(), newDir)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(legacyDir, newDir); err != nil {
			return fmt.Errorf("move legacy task directory %s into tasks: %w", entry.Name(), err)
		}
	}
	return nil
}

func hasTaskFiles(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "description.md")); err == nil && !info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(dir, "notes")); err == nil && info.IsDir() {
		return true
	}
	return false
}

func (w *Workspace) TaskDirectories() ([]string, error) {
	entries, err := os.ReadDir(w.TasksRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, w.TaskPath(entry.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func (w *Workspace) FindTask(identifier string) (string, map[string]any, error) {
	dirs, err := w.TaskDirectories()
	if err != nil {
		return "", nil, err
	}
	for _, dir := range dirs {
		metadata, err := ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		if filepath.Base(dir) == identifier || stringValue(metadata["number"]) == identifier || stringValue(metadata["id"]) == identifier {
			return dir, metadata, nil
		}
	}
	return "", nil, fmt.Errorf("task %q was not found in this workspace", identifier)
}

func (w *Workspace) FindByRemoteID(id string) (string, map[string]any, error) {
	dirs, err := w.TaskDirectories()
	if err != nil {
		return "", nil, err
	}
	for _, dir := range dirs {
		metadata, err := ReadMap(filepath.Join(dir, "metadata.json"))
		if err == nil && stringValue(metadata["id"]) == id {
			return dir, metadata, nil
		}
	}
	return "", nil, os.ErrNotExist
}

func (w *Workspace) ReadState(key string) (State, error) {
	path := w.statePath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	err = json.Unmarshal(data, &state)
	return state, err
}

func (w *Workspace) WriteState(key string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(w.statePath(key), data, 0o600)
}

func (w *Workspace) statePath(key string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(key))
	return filepath.Join(w.Root, ".panda", "state", encoded+".json")
}

func (w *Workspace) StatePathForKey(key string) string {
	return w.statePath(key)
}

func ReadMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if value == nil {
		return nil, fmt.Errorf("%s must contain a JSON object", path)
	}
	return value, nil
}

func WriteMap(path string, value map[string]any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'), 0o600)
}

func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".panda-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func MetadataFromRemote(remote map[string]any) map[string]any {
	metadata := make(map[string]any, len(remote))
	for key, value := range remote {
		if key != "description" && key != "notes" {
			metadata[key] = value
		}
	}
	return metadata
}

func DirectoryName(task map[string]any) string {
	if number := task["number"]; number != nil {
		return fmt.Sprintf("%v", number)
	}
	return safeSegment(stringValue(task["id"]))
}

func WorkspacePath(projectName string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "panda-dashboard", "workspaces", safeSegment(projectName)), nil
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	result := strings.Trim(builder.String(), ".-_")
	if result == "" {
		return "task"
	}
	return result
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
