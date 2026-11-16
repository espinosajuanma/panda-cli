package sync

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
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type RemoteNote struct {
	Filename     string
	HTML         string
	Markdown     string
	Timestamp    any
	AddedByID    string
	AddedByEmail string
	Raw          map[string]any
}

func AdvanceNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	if len(conflicts) == 0 {
		return remote
	}
	baseNotes, err := GetRemoteNotes(baseline)
	if err != nil {
		return remote
	}
	conflicted := make(map[string]map[string]any, len(conflicts))
	for _, filename := range conflicts {
		for _, note := range baseNotes {
			if note.Filename == filename {
				conflicted[filename] = note.Raw
				break
			}
		}
	}
	if len(conflicted) == 0 {
		return remote
	}
	remoteNotes, _ := remote["notes"].([]any)
	current, err := GetRemoteNotes(remote)
	if err != nil {
		return remote
	}
	updatedNotes := make([]any, 0, len(remoteNotes))
	retained := make(map[string]bool, len(conflicted))
	for index, item := range remoteNotes {
		if index < len(current) {
			if baseNote, ok := conflicted[current[index].Filename]; ok {
				updatedNotes = append(updatedNotes, baseNote)
				retained[current[index].Filename] = true
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
	advanced := jsonutil.CloneMap(remote)
	advanced["notes"] = updatedNotes
	if version, exists := baseline["version"]; exists {
		advanced["version"] = version
	} else {
		delete(advanced, "version")
	}
	return advanced
}

func WriteRemoteToLocal(dir string, remote map[string]any) error {
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		return err
	}
	markdown, err := content.HTMLToMarkdown(jsonutil.StringField(remote, "description"))
	if err != nil {
		return err
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600); err != nil {
		return err
	}
	return WriteReleaseNotesToLocal(dir, remote)
}

func BuildMyNotesPayload(ctx context.Context, client *solutions.Client, dir string, remote, baseline map[string]any) ([]map[string]any, error) {
	if client.Email == "" {
		return nil, errors.New("your login email is not saved; run `panda login` to enable editing your notes")
	}
	user, err := client.FindUser(ctx, client.Email)
	if err != nil {
		return nil, err
	}
	changes, err := LocalNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	remoteNotes, err := GetRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	myNotes := make([]map[string]any, 0)
	matchedChanges := make(map[string]bool)
	for _, note := range remoteNotes {
		if !NoteOwnedBy(note, user, client.Email) {
			continue
		}
		body := note.HTML
		if markdown, changed := changes[note.Filename]; changed {
			body, err = content.MarkdownToHTML(markdown)
			if err != nil {
				return nil, fmt.Errorf("convert %s: %w", note.Filename, err)
			}
			matchedChanges[note.Filename] = true
		}
		timestamp, ok := TimestampMillis(note.Timestamp)
		if !ok {
			return nil, fmt.Errorf("note %s has no valid epoch-millisecond timestamp", note.Filename)
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

func PushStagedNotes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, local map[string]any, key string, state workspace.State) (workspace.State, error) {
	id := jsonutil.StringField(local, "id")
	for len(state.StagedNotes) > 0 {
		filename := state.StagedNotes[0]
		markdown, exists, err := workspace.ReadNoteFile(dir, filename)
		if err != nil {
			return state, err
		}
		if !exists {
			return state, fmt.Errorf("read staged note %q: %w", filename, os.ErrNotExist)
		}
		html, err := content.MarkdownToHTML(markdown)
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
		state.Remote = jsonutil.CloneMap(updated)
		if err := SyncRemoteNotes(dir, updated, state.StagedNotes, nil, previous); err != nil {
			return state, err
		}
		_ = workspace.RemoveNoteFile(dir, filename)
		if err := ws.WriteState(key, state); err != nil {
			return state, err
		}
	}
	return state, ws.WriteState(key, state)
}

func GetRemoteNotes(remote map[string]any) ([]RemoteNote, error) {
	items, _ := remote["notes"].([]any)
	counts := make(map[string]int)
	notes := make([]RemoteNote, 0, len(items))
	for _, item := range items {
		note, ok := item.(map[string]any)
		if !ok {
			continue
		}
		html := jsonutil.StringField(note, "note")
		timestamp := time.Unix(0, 0)
		timestampValue := note["timestamp"]
		if millis, ok := TimestampMillis(timestampValue); ok && millis > 0 {
			timestamp = time.UnixMilli(millis)
		}
		author := "unknown"
		addedByID, addedByEmail := "", strings.TrimSpace(jsonutil.StringField(note, "addedByEmail"))
		if addedBy, ok := note["addedBy"].(map[string]any); ok {
			addedByID = jsonutil.StringField(addedBy, "id")
			if jsonutil.StringField(addedBy, "label") != "" {
				author = jsonutil.StringField(addedBy, "label")
			}
		}
		base := workspace.NoteFilename(timestamp, author, 1)
		counts[base]++
		markdown, err := content.HTMLToMarkdown(html)
		if err != nil {
			return nil, err
		}
		notes = append(notes, RemoteNote{
			Filename: workspace.NoteFilename(timestamp, author, counts[base]),
			HTML:     html, Markdown: markdown, Timestamp: timestampValue,
			AddedByID: addedByID, AddedByEmail: addedByEmail,
			Raw: note,
		})
	}
	return notes, nil
}

func TimestampMillis(value any) (int64, bool) {
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

func NoteOwnedBy(note RemoteNote, user map[string]any, email string) bool {
	if note.AddedByID != "" {
		return note.AddedByID == jsonutil.StringField(user, "id")
	}
	return note.AddedByEmail != "" && strings.EqualFold(note.AddedByEmail, email)
}

func LocalNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	notes, err := GetRemoteNotes(baseline)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for _, note := range notes {
		data, exists, err := workspace.ReadNoteFile(dir, note.Filename)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if data != note.Markdown {
			changed[note.Filename] = data
		}
	}
	return changed, nil
}

func LocalNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	localChanges, err := LocalNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	if len(localChanges) == 0 {
		return nil, nil
	}
	baselineNotes, err := GetRemoteNotes(baseline)
	if err != nil {
		return nil, err
	}
	remoteNotes, err := GetRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	baseByFilename := make(map[string]RemoteNote, len(baselineNotes))
	remoteByFilename := make(map[string]RemoteNote, len(remoteNotes))
	for _, note := range baselineNotes {
		baseByFilename[note.Filename] = note
	}
	for _, note := range remoteNotes {
		remoteByFilename[note.Filename] = note
	}
	var conflicts []string
	for filename, localMarkdown := range localChanges {
		base, exists := baseByFilename[filename]
		current, stillExists := remoteByFilename[filename]
		if !exists || !stillExists || (current.Markdown != base.Markdown && current.Markdown != localMarkdown) {
			conflicts = append(conflicts, filename)
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func ChangedNoteFilenames(dir string, baseline map[string]any) ([]string, error) {
	changes, err := LocalNoteChanges(dir, baseline)
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

func SyncRemoteNotes(dir string, remote map[string]any, stagedNotes, preservedNotes []string, previous map[string]any) error {
	notes, err := GetRemoteNotes(remote)
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
		if staged[note.Filename] || preserved[note.Filename] {
			continue
		}
		if err := workspace.WriteNoteFile(dir, note.Filename, note.Markdown); err != nil {
			return err
		}
	}
	current := make(map[string]bool, len(notes))
	for _, note := range notes {
		current[note.Filename] = true
	}
	previousNotes, err := GetRemoteNotes(previous)
	if err != nil {
		return err
	}
	for _, note := range previousNotes {
		if current[note.Filename] || staged[note.Filename] || preserved[note.Filename] {
			continue
		}
		if err := workspace.RemoveNoteFile(dir, note.Filename); err != nil {
			return err
		}
	}
	return nil
}

func BaselineNoteMarkdown(remote map[string]any) (map[string]string, error) {
	notes, err := GetRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(notes))
	for _, note := range notes {
		result[note.Filename] = note.Markdown
	}
	return result, nil
}
