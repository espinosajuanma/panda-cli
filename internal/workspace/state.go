package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
)

func StateKey(dir string, metadata map[string]any) string {
	if id := jsonutil.StringField(metadata, "id"); id != "" {
		return id
	}
	return filepath.Base(dir)
}

func TaskStateKeys(dir string, metadata map[string]any) []string {
	if id := jsonutil.StringField(metadata, "id"); id != "" {
		return []string{id}
	}
	keys := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	for _, key := range []string{jsonutil.StringField(metadata, "localId"), filepath.Base(dir)} {
		if key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

func IsLocalDraft(dir string, metadata map[string]any) bool {
	if jsonutil.StringField(metadata, "id") != "" {
		return false
	}
	if jsonutil.StringField(metadata, "localId") != "" {
		return true
	}
	base := filepath.Base(dir)
	if strings.HasPrefix(base, "draft-") {
		number, err := strconv.Atoi(strings.TrimPrefix(base, "draft-"))
		return err == nil && number > 0
	}
	return strings.HasPrefix(base, "local-") && len(base) > len("local-")
}

func (w *Workspace) LoadTaskState(dir string, metadata map[string]any) (State, string, error) {
	keys := TaskStateKeys(dir, metadata)
	for _, key := range keys {
		state, err := w.ReadState(key)
		if err == nil {
			if IsLocalDraft(dir, metadata) && jsonutil.StringField(state.Remote, "id") == "" {
				state.LocalCreated = true
			}
			return state, key, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return State{}, "", fmt.Errorf("read sync baseline for task %s: %w", task.Label(metadata), err)
		}
	}
	if IsLocalDraft(dir, metadata) {
		return State{LocalCreated: true}, StateKey(dir, metadata), nil
	}
	return State{}, "", fmt.Errorf("task %s has no sync baseline; run `panda pull` before cleaning or pushing", task.Label(metadata))
}

func (w *Workspace) RemoveTaskStateFiles(dir string, metadata map[string]any) {
	for _, key := range TaskStateKeys(dir, metadata) {
		_ = os.Remove(w.StatePathForKey(key))
	}
}

func NoteFilename(timestamp time.Time, author string, duplicateIndex int) string {
	suffix := ""
	if duplicateIndex > 1 {
		suffix = fmt.Sprintf("-%d", duplicateIndex)
	}
	return timestamp.UTC().Format("20060102T150405.000Z") + "-" + noteAuthorSegment(author) + suffix + ".md"
}

func UniqueNoteFilename(notesDir string, timestamp time.Time, author string) (string, error) {
	for index := 1; ; index++ {
		filename := NoteFilename(timestamp, author, index)
		_, err := os.Stat(filepath.Join(notesDir, filename))
		if errors.Is(err, os.ErrNotExist) {
			return filename, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func noteAuthorSegment(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else if r == ' ' {
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
