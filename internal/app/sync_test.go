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
	"reflect"
	"strings"
	"testing"
	"time"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/content"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

func TestTaskConflictsOnlyWhenBothSidesChangedSameField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("same description"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := map[string]any{
		"title":       "Original title",
		"status":      "new",
		"description": "<p>same description</p>",
	}
	local := map[string]any{"title": "Local title", "status": "new"}
	remote := map[string]any{
		"title":       "Original title",
		"status":      "inProgress",
		"description": "<p>same description</p>",
	}
	if conflicts := taskConflicts(local, baseline, remote, dir); len(conflicts) != 0 {
		t.Fatalf("independent edits should merge, got conflicts %v", conflicts)
	}
	remote["title"] = "Remote title"
	conflicts := taskConflicts(local, baseline, remote, dir)
	if !reflect.DeepEqual(conflicts, []string{"title"}) {
		t.Fatalf("got conflicts %v, want [title]", conflicts)
	}
}

func TestAdvanceBaselineKeepsOldValuesOnlyForConflicts(t *testing.T) {
	baseline := map[string]any{
		"title":  "old title",
		"status": "new",
	}
	remote := map[string]any{
		"id":     "task-1",
		"title":  "remote title",
		"status": "inProgress",
	}
	advanced := advanceBaseline(baseline, remote, []string{"title"}, false)
	if advanced["title"] != "old title" {
		t.Fatalf("conflicted baseline title = %v", advanced["title"])
	}
	if advanced["status"] != "inProgress" {
		t.Fatalf("non-conflicted remote update was not advanced: %v", advanced["status"])
	}
	if advanced["id"] != "task-1" {
		t.Fatalf("remote-only field was not advanced: %#v", advanced)
	}
}

func TestMergeForPushPreservesFreshRemoteFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("original body"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := map[string]any{"title": "Edited title", "status": "new"}
	baseline := map[string]any{
		"title":       "Old title",
		"status":      "new",
		"description": "<p>original body</p>",
	}
	remote := map[string]any{
		"id":          "task-9",
		"title":       "Old title",
		"status":      "inProgress",
		"description": "<p>original body</p>",
		"package":     map[string]any{"id": "package-1"},
	}
	merged, changed, err := mergeForPush(local, remote, baseline, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected local title update")
	}
	if merged["title"] != "Edited title" || merged["status"] != "inProgress" {
		t.Fatalf("unexpected merged task: %#v", merged)
	}
	if !reflect.DeepEqual(merged["package"], remote["package"]) {
		t.Fatalf("fresh remote fields were lost: %#v", merged)
	}
}

func TestPullThenPushMergesIndependentRemoteAndLocalChanges(t *testing.T) {
	remote := map[string]any{
		"id":          "task-1",
		"number":      float64(14),
		"title":       "Original",
		"type":        "feature",
		"priority":    "medium",
		"complexity":  "s",
		"status":      "new",
		"description": "<p>Original body</p>",
		"project":     map[string]any{"id": "project-1", "label": "Alpha"},
		"labels":      []any{},
		"assignees":   []any{},
		"reviewers":   []any{},
		"notes":       []any{},
		"package":     map[string]any{"id": "package-1"},
	}
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("token") != "test-token" {
			t.Errorf("missing API token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks":
			if r.URL.Query().Get("project.id") != "project-1" {
				t.Errorf("task list missing project filter: %s", r.URL.RawQuery)
			}
			_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			// Simulate a remote-only change after pull. The local title edit should
			// merge without reverting this status update.
			remote["status"] = "inProgress"
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1":
			putCount++
			if err := json.NewDecoder(r.Body).Decode(&remote); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	client := solutions.NewClient(server.URL, "test-token")
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := pull(context.Background(), client, ws, project, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws.Root, "14")
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["title"] = "Locally edited"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("Updated **body**"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), client, ws, project, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if remote["title"] != "Locally edited" || remote["status"] != "inProgress" {
		t.Fatalf("independent updates were not merged: %#v", remote)
	}
	if !strings.Contains(stringField(remote, "description"), "Updated ") || putCount != 1 {
		t.Fatalf("local description was not pushed: puts=%d, description=%q", putCount, stringField(remote, "description"))
	}
	if !reflect.DeepEqual(remote["package"], map[string]any{"id": "package-1"}) {
		t.Fatalf("unmodified API fields were not preserved: %#v", remote["package"])
	}
}

func TestInteractiveEditValidatesEnumsAndWritesMetadata(t *testing.T) {
	root := t.TempDir()
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "number": float64(14), "title": "Original", "type": "feature", "priority": "medium", "complexity": "s"}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("Changed title\n\nhigh\n-\nn\n\n")
	if err := editTask(context.Background(), ws, []string{"14"}, solutions.NewClient("http://unused", ""), &config.Config{}, input, io.Discard); err != nil {
		t.Fatal(err)
	}
	updated, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if updated["title"] != "Changed title" || updated["priority"] != "high" {
		t.Fatalf("interactive values were not saved: %#v", updated)
	}
	if _, exists := updated["complexity"]; exists {
		t.Fatalf("complexity was not cleared: %#v", updated["complexity"])
	}
}

func TestInteractiveEditRejectsInvalidEnumWithoutSaving(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"title": "Original", "type": "feature", "priority": "medium"}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	err := editMetadataInteractive(dir, metadata, strings.NewReader("Changed\ninvalid\n"), io.Discard)
	if err == nil {
		t.Fatal("expected invalid type error")
	}
	actual, readErr := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if actual["title"] != "Original" {
		t.Fatalf("partial invalid edit was saved: %#v", actual)
	}
}

func TestInteractiveLabelPickerAddsAndRemovesAPIRelationships(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "labels": []any{}}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/dev.labels" || r.URL.Query().Get("project.id") != "project-1" {
			t.Errorf("unexpected label request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":2,"items":[{"id":"l1","name":"Feature A"},{"id":"l2","name":"Feature B"}]}`))
	}))
	defer server.Close()
	client := solutions.NewClient(server.URL, "token")
	if err := editLabels(context.Background(), client, "project-1", dir, metadata, []string{"Feature A"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ := metadata["labels"].([]any)
	if len(labels) != 1 || stringField(labels[0].(map[string]any), "id") != "l1" {
		t.Fatalf("label relationship not saved: %#v", metadata["labels"])
	}
	if err := editLabels(context.Background(), client, "project-1", dir, metadata, []string{"--remove=Feature A"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ = metadata["labels"].([]any)
	if len(labels) != 0 {
		t.Fatalf("label relationship not removed: %#v", labels)
	}
}

func TestInteractiveLabelPickerSearchesPrefixes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "labels": []any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":2,"items":[{"id":"l1","name":"Feature A"},{"id":"l2","name":"Fix"}]}`))
	}))
	defer server.Close()
	input := strings.NewReader("fe\n1\n\n")
	var output strings.Builder
	if err := editLabels(context.Background(), solutions.NewClient(server.URL, "token"), "project-1", dir, metadata, nil, input, &output); err != nil {
		t.Fatal(err)
	}
	updated, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ := updated["labels"].([]any)
	if len(labels) != 1 || stringField(labels[0].(map[string]any), "label") != "Feature A" {
		t.Fatalf("prefix selection did not add label: %#v", updated["labels"])
	}
	if !strings.Contains(output.String(), "Feature A") {
		t.Fatalf("prefix suggestions missing from UI: %q", output.String())
	}
}

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
		"id": "task-1", "number": float64(14), "title": "Title", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Body</p>",
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
			remote["notes"] = []any{map[string]any{
				"note": payload["note"], "timestamp": float64(1_700_000_000_000),
				"addedBy": map[string]any{"id": "user-1", "label": "Developer"},
			}}
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
	if addNoteCount != 1 {
		t.Fatalf("AddNote was called %d times, want once", addNoteCount)
	}
	if !strings.Contains(addedNote, "New note") {
		t.Fatalf("Markdown note was not converted and sent: %q", addedNote)
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
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "New note" {
		t.Fatalf("server note was not converted back to Markdown: %q", content)
	}
}

func TestCleanRestoresSyncedTaskAndRemovesPendingNote(t *testing.T) {
	remote := map[string]any{
		"id": "task-1", "number": float64(14), "title": "Original", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Original body</p>",
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
	state.StagedNotes = []string{filename}
	if err := ws.WriteState("task-1", state); err != nil {
		t.Fatal(err)
	}
	if err := clean(ws, []string{"14"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	cleaned, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cleaned["title"] != "Original" {
		t.Fatalf("metadata not restored: %#v", cleaned)
	}
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(description) != "Original body" {
		t.Fatalf("description not restored: %q", description)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("pending note not removed: %#v", entries)
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
		created["id"] = "task-remote-15"
		created["number"] = float64(15)
		created["label"] = "15"
		created["version"] = float64(1)
		created["notes"] = []any{}
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
	finalDir := filepath.Join(ws.Root, "15")
	metadata, err := workspace.ReadMap(filepath.Join(finalDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if stringField(metadata, "id") != "task-remote-15" || stringField(metadata, "title") != "New task" {
		t.Fatalf("remote identity was not applied locally: %#v", metadata)
	}
	state, err := ws.ReadState("task-remote-15")
	if err != nil || state.LocalCreated {
		t.Fatalf("local task did not become synced: state=%#v err=%v", state, err)
	}
}

func TestPushContactsOnlyTasksWithLocalChanges(t *testing.T) {
	cleanTask := map[string]any{
		"id": "task-clean", "number": float64(14), "title": "Unchanged", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	dirtyTask := map[string]any{
		"id": "task-dirty", "number": float64(15), "title": "Original", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
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

func TestListGroupsByWorkflowStatusSortsByRankAndFilters(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("new-b", 12, "new", "b", "m", "New B", "feature"),
		listTask("new-a", 11, "new", "a", nil, "New A", "feature"),
		listTask("todo", 13, "toDo", "c", "", "To Do", "bug"),
		listTask("progress", 14, "inProgress", "d", "s", "In Progress", "feature"),
		listTask("review", 15, "inReview", "e", "m", "In Review", "feature"),
		listTask("complete", 16, "completed", "f", "l", "Completed", "feature"),
		listTask("staging", 17, "staging", "g", "xl", "Staging", "feature"),
		listTask("release", 18, "released", "h", "xxl", "Release", "release"),
		listTask("discard", 19, "discarded", "i", "s", "Discarded", "bug"),
	}
	for _, task := range fixtures {
		addSyncedTask(t, ws, task)
	}

	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	statusPositions := []int{
		strings.Index(text, "new\n"),
		strings.Index(text, "toDo\n"),
		strings.Index(text, "inProgress\n"),
		strings.Index(text, "inReview\n"),
		strings.Index(text, "completed\n"),
		strings.Index(text, "staging\n"),
		strings.Index(text, "released\n"),
	}
	for i, position := range statusPositions {
		if position < 0 || (i > 0 && position <= statusPositions[i-1]) {
			t.Fatalf("workflow statuses are not grouped in the requested order: %q", text)
		}
	}
	if strings.Contains(text, "Discarded") {
		t.Fatalf("discarded task appeared without an explicit status filter: %q", text)
	}
	if strings.Index(text, "New A") > strings.Index(text, "New B") {
		t.Fatalf("new tasks were not sorted by rank: %q", text)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--status", "new,toDo", "--no-complexity"}, &output); err != nil {
		t.Fatal(err)
	}
	filtered := output.String()
	if !strings.Contains(filtered, "New A") || !strings.Contains(filtered, "To Do") || strings.Contains(filtered, "New B") || strings.Contains(filtered, "Discarded") {
		t.Fatalf("status/no-complexity filters were not applied: %q", filtered)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--status=discarded"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Discarded") {
		t.Fatalf("explicit discarded filter did not show discarded tasks: %q", output.String())
	}
}

func listTask(id string, number float64, status, rank string, complexity any, title, taskType string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "title": title, "type": taskType,
		"priority": "medium", "complexity": complexity, "status": status, "rank": rank,
		"description": "", "project": map[string]any{"id": "project-1"},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
}

func makeSyncedTask(t *testing.T, remote map[string]any) (*workspace.Workspace, string) {
	t.Helper()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := addSyncedTask(t, ws, remote)
	return ws, dir
}

func addSyncedTask(t *testing.T, ws *workspace.Workspace, remote map[string]any) string {
	t.Helper()
	dir := filepath.Join(ws.Root, workspace.DirectoryName(remote))
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		t.Fatal(err)
	}
	markdown, err := content.HTMLToMarkdown(stringField(remote, "description"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}
	id := stringField(remote, "id")
	if err := ws.WriteState(id, workspace.State{Remote: cloneMap(remote)}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func marshalForTest(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
