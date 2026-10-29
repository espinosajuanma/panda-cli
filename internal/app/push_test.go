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

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

func TestNoteFilenameDisambiguatesSameTimestampAndAuthor(t *testing.T) {
	stamp := time.Unix(1_700_000_000, 0)
	first := noteFilename(stamp, "A Person", 1)
	second := noteFilename(stamp, "A Person", 2)
	if first == second || !strings.Contains(second, "-2.md") {
		t.Fatalf("duplicate note filenames are not unique: %q %q", first, second)
	}
}

func TestPushDoesNotOverwriteAnOverlappingRemoteEdit(t *testing.T) {
	remote := map[string]any{
		"id": "task-1", "number": float64(14), "title": "Remote title", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(remote)
		case http.MethodPut:
			putCount++
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, map[string]any{
		"id": "task-1", "number": float64(14), "title": "Original title", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	})
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	local["title"] = "Local title"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err == nil {
		t.Fatal("expected a conflict")
	}
	if putCount != 0 {
		t.Fatalf("conflicting push made %d update request(s)", putCount)
	}
}

func TestPushSendsStagedNotesThroughAddNoteAction(t *testing.T) {
	remote := map[string]any{
		"id": "task-1", "number": float64(14), "title": "Title", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	addNoteCount := 0
	var addedNote string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1/addNote":
			addNoteCount++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			addedNote = payload["note"]
			remote["notes"] = []any{map[string]any{"note": payload["note"], "timestamp": float64(1_700_000_000_000), "addedBy": map[string]any{"id": "user-1", "label": "Developer"}}}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	state, err := ws.ReadState("task-1")
	if err != nil {
		t.Fatal(err)
	}
	filename := "20261027T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("New note"), 0o600); err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{filename}
	if err := ws.WriteState("task-1", state); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if addNoteCount != 1 || !strings.Contains(addedNote, "New note") {
		t.Fatalf("AddNote calls=%d; note payload=%q", addNoteCount, addedNote)
	}
	state, err = ws.ReadState("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.StagedNotes) != 0 {
		t.Fatalf("staged note was not cleared: %#v", state.StagedNotes)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes", filename)); !os.IsNotExist(err) {
		t.Fatalf("local pending note remained after successful push: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Name(), "Developer") {
		t.Fatalf("authoritative server note was not written locally: %#v", entries)
	}
	content, err := os.ReadFile(filepath.Join(dir, "notes", entries[0].Name()))
	if err != nil || string(content) != "New note" {
		t.Fatalf("server note content = %q, error = %v", content, err)
	}
}

func TestPushAppliesStagedDiscardThroughSolutionsAction(t *testing.T) {
	remote := map[string]any{
		"id": "task-1", "number": float64(14), "title": "No longer needed", "type": "feature", "priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	discardCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages"):
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1/discard":
			discardCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body) != 0 {
				t.Errorf("discard action payload = %#v, want empty object", body)
			}
			remote["status"] = "discarded"
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	if err := discard(ws, []string{"14"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if discardCalls != 1 || stringField(remote, "status") != "discarded" {
		t.Fatalf("discard action was not applied: calls=%d remote=%#v", discardCalls, remote)
	}
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil || stringField(local, "status") != "discarded" {
		t.Fatalf("local task status not synchronized: %#v, error=%v", local, err)
	}
	state, err := ws.ReadState("task-1")
	if err != nil || state.StagedDiscard || stringField(state.Remote, "status") != "discarded" {
		t.Fatalf("discard staging was not cleared: %#v, error=%v", state, err)
	}
}

func TestCleanRestoresSyncedTaskAndRemovesPendingNote(t *testing.T) {
	remote := map[string]any{
		"id": "task-1", "number": float64(14), "title": "Original", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Original body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	ws, dir := makeSyncedTask(t, remote)
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	local["title"] = "Changed"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("Changed body"), 0o600); err != nil {
		t.Fatal(err)
	}
	filename := "20261027T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("Pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("task-1")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedDiscard = true
	local["status"] = "discarded"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{filename}
	if err := ws.WriteState("task-1", state); err != nil {
		t.Fatal(err)
	}
	if err := clean(ws, []string{"14"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	cleaned, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil || cleaned["title"] != "Original" || cleaned["status"] != "new" {
		t.Fatalf("synced metadata not restored: %#v, error=%v", cleaned, err)
	}
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil || string(description) != "Original body" {
		t.Fatalf("description = %q, error=%v", description, err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "notes"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("pending note not removed: %#v, error=%v", entries, err)
	}
	state, err = ws.ReadState("task-1")
	if err != nil || state.StagedDiscard {
		t.Fatalf("clean left staged discard behind: %#v, error=%v", state, err)
	}
}

func TestNewTaskGetsRemoteNumberAfterSuccessfulPush(t *testing.T) {
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/data/dev.tasks" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Error(err)
		}
		created["id"], created["number"], created["label"], created["version"], created["notes"] = "task-remote-15", float64(15), "15", float64(1), []any{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(created)
	}))
	defer server.Close()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := createTask(ws, project, []string{"New", "task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("local task was not created: dirs=%v err=%v", dirs, err)
	}
	if err := os.WriteFile(filepath.Join(dirs[0], "description.md"), []byte("Hello **world**"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	if created["project"].(map[string]any)["id"] != "project-1" || !strings.Contains(stringField(created, "description"), "<strong>world</strong>") {
		t.Fatalf("remote task payload incorrect: %#v", created)
	}
	finalDir := ws.TaskPath("15")
	metadata, err := workspace.ReadMap(filepath.Join(finalDir, "metadata.json"))
	if err != nil || stringField(metadata, "id") != "task-remote-15" || stringField(metadata, "title") != "New task" {
		t.Fatalf("remote identity not applied locally: %#v, error=%v", metadata, err)
	}
	state, err := ws.ReadState("task-remote-15")
	if err != nil || state.LocalCreated {
		t.Fatalf("local task did not become synced: state=%#v err=%v", state, err)
	}
}

func TestPushContactsOnlyTasksWithLocalChanges(t *testing.T) {
	cleanTask := map[string]any{"id": "task-clean", "number": float64(14), "title": "Unchanged", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Body</p>", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}}
	dirtyTask := map[string]any{"id": "task-dirty", "number": float64(15), "title": "Original", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Body</p>", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}}
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-dirty":
			_ = json.NewEncoder(w).Encode(dirtyTask)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-dirty":
			if err := json.NewDecoder(r.Body).Decode(&dirtyTask); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(dirtyTask)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	cleanDir := addSyncedTask(t, ws, cleanTask)
	dirtyDir := addSyncedTask(t, ws, dirtyTask)
	metadata, err := workspace.ReadMap(filepath.Join(dirtyDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["title"] = "Locally edited"
	if err := workspace.WriteMap(filepath.Join(dirtyDir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, &output); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0] != "GET /data/dev.tasks/task-dirty" || requests[1] != "PUT /data/dev.tasks/task-dirty" {
		t.Fatalf("unexpected API calls; clean task path=%s, requests=%v", cleanDir, requests)
	}
	if strings.Contains(output.String(), "Pushed 14") || !strings.Contains(output.String(), "Pushed 15") {
		t.Fatalf("push output included clean tasks or omitted dirty task: %q", output.String())
	}
}
