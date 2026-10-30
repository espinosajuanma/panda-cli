package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func advanceNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	if len(conflicts) == 0 {
		return remote
	}
	baseNotes, err := getRemoteNotes(baseline)
	if err != nil {
		return remote
	}
	conflicted := make(map[string]map[string]any, len(conflicts))
	for _, filename := range conflicts {
		for _, note := range baseNotes {
			if note.filename == filename {
				conflicted[filename] = note.raw
				break
			}
		}
	}
	if len(conflicted) == 0 {
		return remote
	}
	remoteNotes, _ := remote["notes"].([]any)
	current, err := getRemoteNotes(remote)
	if err != nil {
		return remote
	}
	updatedNotes := make([]any, 0, len(remoteNotes))
	retained := make(map[string]bool, len(conflicted))
	for index, item := range remoteNotes {
		if index < len(current) {
			if baseNote, ok := conflicted[current[index].filename]; ok {
				updatedNotes = append(updatedNotes, baseNote)
				retained[current[index].filename] = true
				continue
			}
		}
		updatedNotes = append(updatedNotes, item)
	}
	for filename, baseNote := range conflicted {
		if !retained[filename] {
			updatedNotes = append(updatedNotes, baseNote)
		}
	}
	advanced := cloneMap(remote)
	advanced["notes"] = updatedNotes
	if version, exists := baseline["version"]; exists {
		advanced["version"] = version
	} else {
		delete(advanced, "version")
	}
	return advanced
}

func writeRemoteToLocal(dir string, remote map[string]any) error {
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		return err
	}
	markdown, err := content.HTMLToMarkdown(stringField(remote, "description"))
	if err != nil {
		return err
	}
	return workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600)
}

func buildMyNotesPayload(ctx context.Context, client *solutions.Client, dir string, remote, baseline map[string]any) ([]map[string]any, error) {
	if client.Email == "" {
		return nil, errors.New("your login email is not saved; run `panda login` to enable editing your notes")
	}
	user, err := client.FindUser(ctx, client.Email)
	if err != nil {
		return nil, err
	}
	changes, err := localNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	remoteNotes, err := getRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	myNotes := make([]map[string]any, 0)
	matchedChanges := make(map[string]bool)
	for _, note := range remoteNotes {
		if !noteOwnedBy(note, user, client.Email) {
			continue
		}
		body := note.html
		if markdown, changed := changes[note.filename]; changed {
			body, err = content.MarkdownToHTML(markdown)
			if err != nil {
				return nil, fmt.Errorf("convert %s: %w", note.filename, err)
			}
			matchedChanges[note.filename] = true
		}
		timestamp, ok := timestampMillis(note.timestamp)
		if !ok {
			return nil, fmt.Errorf("note %s has no valid epoch-millisecond timestamp", note.filename)
		}
		myNotes = append(myNotes, map[string]any{"note": body, "timestamp": timestamp})
	}
	for filename := range changes {
		if !matchedChanges[filename] {
			return nil, fmt.Errorf("%s is not one of the current user's notes; only your own notes can be edited", filename)
		}
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if len(myNotes) == 0 {
		return nil, errors.New("the current user has no notes to edit on this task")
	}
	return myNotes, nil
}

func pushStagedNotes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, local map[string]any, key string, state workspace.State) (workspace.State, error) {
	id := stringField(local, "id")
	for len(state.StagedNotes) > 0 {
		filename := state.StagedNotes[0]
		markdown, err := os.ReadFile(filepath.Join(dir, "notes", filename))
		if err != nil {
			return state, err
		}
		html, err := content.MarkdownToHTML(string(markdown))
		if err != nil {
			return state, err
		}
		if err := client.AddNote(ctx, id, html); err != nil {
			return state, err
		}
		state.StagedNotes = state.StagedNotes[1:]
		if err := ws.WriteState(key, state); err != nil {
			return state, err
		}
		updated, err := client.GetTask(ctx, id)
		if err != nil {
			return state, err
		}
		previous := state.Remote
		state.Remote = cloneMap(updated)
		if err := syncRemoteNotes(dir, updated, state.StagedNotes, nil, previous); err != nil {
			return state, err
		}
		_ = os.Remove(filepath.Join(dir, "notes", filename))
		if err := ws.WriteState(key, state); err != nil {
			return state, err
		}
	}
	return state, ws.WriteState(key, state)
}

type remoteNote struct {
	filename     string
	html         string
	markdown     string
	timestamp    any
	addedByID    string
	addedByEmail string
	raw          map[string]any
}

func getRemoteNotes(remote map[string]any) ([]remoteNote, error) {
	items, _ := remote["notes"].([]any)
	counts := make(map[string]int)
	notes := make([]remoteNote, 0, len(items))
	for _, item := range items {
		note, ok := item.(map[string]any)
		if !ok {
			continue
		}
		html := stringField(note, "note")
		timestamp := time.Unix(0, 0)
		timestampValue := note["timestamp"]
		if millis, ok := timestampMillis(timestampValue); ok && millis > 0 {
			timestamp = time.UnixMilli(millis)
		}
		author := "unknown"
		addedByID, addedByEmail := "", strings.TrimSpace(stringField(note, "addedByEmail"))
		if addedBy, ok := note["addedBy"].(map[string]any); ok {
			addedByID = stringField(addedBy, "id")
			if stringField(addedBy, "label") != "" {
				author = stringField(addedBy, "label")
			}
		}
		base := noteFilename(timestamp, author, 1)
		counts[base]++
		markdown, err := content.HTMLToMarkdown(html)
		if err != nil {
			return nil, err
		}
		notes = append(notes, remoteNote{
			filename: noteFilename(timestamp, author, counts[base]),
			html:     html, markdown: markdown, timestamp: timestampValue,
			addedByID: addedByID, addedByEmail: addedByEmail,
			raw: note,
		})
	}
	return notes, nil
}

func timestampMillis(value any) (int64, bool) {
	switch timestamp := value.(type) {
	case float64:
		return int64(timestamp), true
	case float32:
		return int64(timestamp), true
	case int64:
		return timestamp, true
	case int:
		return int64(timestamp), true
	case json.Number:
		millis, err := timestamp.Int64()
		return millis, err == nil
	case string:
		millis, err := strconv.ParseInt(timestamp, 10, 64)
		return millis, err == nil
	default:
		return 0, false
	}
}

func noteOwnedBy(note remoteNote, user map[string]any, email string) bool {
	if note.addedByID != "" {
		return note.addedByID == stringField(user, "id")
	}
	return note.addedByEmail != "" && strings.EqualFold(note.addedByEmail, email)
}

func localNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	notes, err := getRemoteNotes(baseline)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for _, note := range notes {
		data, err := os.ReadFile(filepath.Join(dir, "notes", note.filename))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if string(data) != note.markdown {
			changed[note.filename] = string(data)
		}
	}
	return changed, nil
}

func localNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	localChanges, err := localNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	if len(localChanges) == 0 {
		return nil, nil
	}
	baselineNotes, err := getRemoteNotes(baseline)
	if err != nil {
		return nil, err
	}
	remoteNotes, err := getRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	baseByFilename := make(map[string]remoteNote, len(baselineNotes))
	remoteByFilename := make(map[string]remoteNote, len(remoteNotes))
	for _, note := range baselineNotes {
		baseByFilename[note.filename] = note
	}
	for _, note := range remoteNotes {
		remoteByFilename[note.filename] = note
	}
	var conflicts []string
	for filename, localMarkdown := range localChanges {
		base, exists := baseByFilename[filename]
		current, stillExists := remoteByFilename[filename]
		if !exists || !stillExists || (current.markdown != base.markdown && current.markdown != localMarkdown) {
			conflicts = append(conflicts, filename)
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func changedNoteFilenames(dir string, baseline map[string]any) ([]string, error) {
	changes, err := localNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, 0, len(changes))
	for filename := range changes {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)
	return filenames, nil
}

func syncRemoteNotes(dir string, remote map[string]any, stagedNotes, preservedNotes []string, previous map[string]any) error {
	notes, err := getRemoteNotes(remote)
	if err != nil {
		return err
	}
	staged := make(map[string]bool, len(stagedNotes))
	for _, filename := range stagedNotes {
		staged[filename] = true
	}
	preserved := make(map[string]bool, len(preservedNotes))
	for _, filename := range preservedNotes {
		preserved[filename] = true
	}
	for _, note := range notes {
		path := filepath.Join(dir, "notes", note.filename)
		if staged[note.filename] || preserved[note.filename] {
			continue
		}
		if err := workspace.WriteFileAtomic(path, []byte(note.markdown), 0o600); err != nil {
			return err
		}
	}
	current := make(map[string]bool, len(notes))
	for _, note := range notes {
		current[note.filename] = true
	}
	previousNotes, err := getRemoteNotes(previous)
	if err != nil {
		return err
	}
	for _, note := range previousNotes {
		if current[note.filename] || staged[note.filename] || preserved[note.filename] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, "notes", note.filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
