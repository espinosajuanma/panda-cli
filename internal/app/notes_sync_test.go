package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestEditNoteOpensOnlyCurrentUsersSyncedNote(t *testing.T) {
	const timestamp = int64(1793188800000)
	remote := map[string]any{"id": "task-note-command", "number": float64(13769), "title": "Edit my note", "type": "feature", "priority": "medium", "status": "new", "description": "", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{map[string]any{"note": "<p>Original note body</p>", "timestamp": float64(timestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/sys.users" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"total":1,"items":[{"id":"user-1","email":"juan@example.com","label":"Juan"}]}`)
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	filename := noteFilename(time.UnixMilli(timestamp), "Juan", 1)
	path := filepath.Join(dir, "notes", filename)
	if err := workspace.WriteFileAtomic(path, []byte("Original note body"), 0o600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'Edited by command' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	client := solutions.NewClient(server.URL, "test-token")
	client.Email = "juan@example.com"
	var output strings.Builder
	if err := editNote(ws, []string{"13769", strings.TrimSuffix(filename, ".md")}, client, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "Edited by command" {
		t.Fatalf("editor wrote %q, error=%v", data, err)
	}
	if !strings.Contains(output.String(), "panda diff --verbose") {
		t.Fatalf("edit command did not print review instructions: %q", output.String())
	}
}

func TestPullPreservesEditedNotesAndReportsOverlappingRemoteEdits(t *testing.T) {
	const timestamp = int64(1793188800000)
	baseline := map[string]any{"id": "task-note-pull", "number": float64(13768), "version": float64(1), "title": "Pull edited notes", "type": "feature", "priority": "medium", "complexity": "s", "status": "new", "description": "", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{map[string]any{"note": "<p>Original note body</p>", "timestamp": float64(timestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}}}}
	remote := cloneMap(baseline)
	remote["version"], remote["status"] = float64(2), "inProgress"
	remote["notes"] = []any{map[string]any{"note": "<p>Remote note edit</p>", "timestamp": float64(timestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/data/projects/project-1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"members":[]}`)
			return
		}
		if r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/data/dev.tasks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, baseline)
	filename := noteFilename(time.UnixMilli(timestamp), "Juan", 1)
	path := filepath.Join(dir, "notes", filename)
	if err := workspace.WriteFileAtomic(path, []byte("My local note edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	client := solutions.NewClient(server.URL, "token")
	if err := pull(context.Background(), client, ws, project, &output); err == nil {
		t.Fatal("pull should report the overlapping note edit as a conflict")
	}
	local, err := os.ReadFile(path)
	if err != nil || string(local) != "My local note edit" {
		t.Fatalf("pull replaced local note edit: %q, error=%v", local, err)
	}
	if !strings.Contains(output.String(), "Conflict task 13768: notes/"+filename) {
		t.Fatalf("pull did not report the overlapping note edit:\n%s", output.String())
	}
	state, err := ws.ReadState("task-note-pull")
	if err != nil {
		t.Fatal(err)
	}
	baselineNotes, err := getRemoteNotes(state.Remote)
	if err != nil || len(baselineNotes) != 1 || baselineNotes[0].markdown != "Original note body" {
		t.Fatalf("conflicted note baseline advanced unexpectedly: %#v, error=%v", baselineNotes, err)
	}
	if state.Remote["version"] != float64(1) {
		t.Fatalf("note-conflict baseline version = %v, want 1", state.Remote["version"])
	}
	if err := clean(ws, []string{"13768"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatalf("pull after cleaning conflicted local edit: %v", err)
	}
	local, err = os.ReadFile(path)
	if err != nil || string(local) != "Remote note edit" {
		t.Fatalf("pull after clean did not take remote note: %q, error=%v", local, err)
	}
}

func TestPushEditsOnlyCurrentUsersNotesWithFullMyNotesPayload(t *testing.T) {
	const ownTimestamp = int64(1793188800000)
	const ownSecondTimestamp = int64(1793188860000)
	const otherTimestamp = int64(1793188920000)
	remote := map[string]any{"id": "task-note-edit", "number": float64(13767), "title": "Edit a synced note", "type": "feature", "priority": "medium", "status": "new", "description": "", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{
		map[string]any{"note": "<p>Original note body</p>", "timestamp": float64(ownTimestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}},
		map[string]any{"note": "<p>Keep this note</p>", "timestamp": float64(ownSecondTimestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}},
		map[string]any{"note": "<p>Someone else's note</p>", "timestamp": float64(otherTimestamp), "addedBy": map[string]any{"id": "user-2", "label": "Other"}},
	}}
	var actionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-note-edit":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodGet && r.URL.Path == "/data/sys.users":
			if r.URL.Query().Get("email") != "juan@example.com" {
				t.Errorf("user lookup email = %q", r.URL.Query().Get("email"))
			}
			_, _ = fmt.Fprint(w, `{"total":1,"items":[{"id":"user-1","email":"juan@example.com","label":"Juan"}]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-note-edit/editMyNotes":
			actionCalls++
			var request struct {
				MyNotes []map[string]any `json:"myNotes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if len(request.MyNotes) != 2 {
				t.Fatalf("myNotes count = %d, want 2", len(request.MyNotes))
			}
			if request.MyNotes[0]["timestamp"] != float64(ownTimestamp) || !strings.Contains(stringField(request.MyNotes[0], "note"), "Updated note body") {
				t.Errorf("edited note payload = %#v", request.MyNotes[0])
			}
			if got := stringField(request.MyNotes[1], "note"); got != "<p>Keep this note</p>" {
				t.Errorf("unchanged note was not preserved: %q", got)
			}
			for _, item := range remote["notes"].([]any) {
				note := item.(map[string]any)
				if timestamp, ok := note["timestamp"].(float64); ok && int64(timestamp) == ownTimestamp {
					note["note"] = request.MyNotes[0]["note"]
				}
			}
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	for _, note := range []struct {
		timestamp    int64
		author, body string
	}{{ownTimestamp, "Juan", "Original note body"}, {ownSecondTimestamp, "Juan", "Keep this note"}, {otherTimestamp, "Other", "Someone else's note"}} {
		filename := noteFilename(time.UnixMilli(note.timestamp), note.author, 1)
		if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte(note.body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	editedFilename := noteFilename(time.UnixMilli(ownTimestamp), "Juan", 1)
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", editedFilename), []byte("Updated note body"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := solutions.NewClient(server.URL, "test-token")
	client.Email = "juan@example.com"
	if err := push(context.Background(), client, ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if actionCalls != 1 {
		t.Fatalf("editMyNotes calls = %d, want 1", actionCalls)
	}
	if got := stringField(remote["notes"].([]any)[2].(map[string]any), "note"); got != "<p>Someone else's note</p>" {
		t.Fatalf("other user's note was modified: %q", got)
	}
	state, err := ws.ReadState("task-note-edit")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stringField(state.Remote["notes"].([]any)[0].(map[string]any), "note"), "Updated note body") {
		t.Fatalf("successful note edit was not saved to baseline: %#v", state.Remote["notes"])
	}
}
