package app

import (
	"bytes"
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
	dir := ws.TaskPath("14")
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

func TestPullSkipsTasksWithUnchangedVersion(t *testing.T) {
	remote := map[string]any{
		"id": "task-versioned", "number": float64(51), "version": float64(1),
		"title": "Original title", "type": "feature", "priority": "medium", "complexity": "s", "status": "new",
		"description": "<p>Body</p>", "project": map[string]any{"id": "project-1"},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/dev.tasks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
	}))
	defer server.Close()

	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	client := solutions.NewClient(server.URL, "token")
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	var output strings.Builder
	if err := pull(context.Background(), client, ws, project, &output); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("51")

	remote["title"] = "Changed without version bump"
	output.Reset()
	if err := pull(context.Background(), client, ws, project, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "0 updated") {
		t.Fatalf("unchanged version was counted as updated: %q", output.String())
	}
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if local["title"] != "Original title" {
		t.Fatalf("unchanged version rewrote local task metadata: %#v", local)
	}

	remote["title"] = "Changed with version bump"
	remote["version"] = float64(2)
	output.Reset()
	if err := pull(context.Background(), client, ws, project, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "1 updated") {
		t.Fatalf("changed version was not counted as updated: %q", output.String())
	}
	local, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if local["title"] != "Changed with version bump" || local["version"] != float64(2) {
		t.Fatalf("versioned remote update was not applied locally: %#v", local)
	}
}

func TestPullProgressBarFormattingAndNonTerminalOutput(t *testing.T) {
	if got, want := formatPullProgress(5, 10), "Pulling tasks: [##########----------] 5/10 (50%)"; got != want {
		t.Fatalf("progress bar = %q, want %q", got, want)
	}
	var output strings.Builder
	progress := newPullProgress(&output, 3)
	progress.Update(0)
	progress.Advance()
	progress.Finish()
	if output.Len() != 0 {
		t.Fatalf("non-terminal output included a progress bar: %q", output.String())
	}
}

func TestInteractiveEditValidatesEnumsAndWritesMetadata(t *testing.T) {
	root := t.TempDir()
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
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
	finalDir := ws.TaskPath("15")
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

func TestDiffGroupsTaskChangesAndShowsDescriptionLines(t *testing.T) {
	first := map[string]any{
		"id": "task-diff-1", "number": float64(13359), "title": "Improve task diffs", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Original body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	second := map[string]any{
		"id": "task-diff-2", "number": float64(13360), "title": "Unchanged task", "type": "feature",
		"priority": "medium", "status": "new", "description": "<p>Same body</p>",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	ws, firstDir := makeSyncedTask(t, first)
	secondDir := addSyncedTask(t, ws, second)
	metadata, err := workspace.ReadMap(filepath.Join(firstDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["priority"] = "high"
	if err := workspace.WriteMap(filepath.Join(firstDir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstDir, "description.md"), []byte("Updated body\nA new line"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondMetadata, err := workspace.ReadMap(filepath.Join(secondDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata["title"] = "Changed second title"
	if err := workspace.WriteMap(filepath.Join(secondDir, "metadata.json"), secondMetadata); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := diff(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	allTasks := output.String()
	firstHeader := strings.Index(allTasks, "Task 13359: Improve task diffs")
	secondHeader := strings.Index(allTasks, "Task 13360: Changed second title")
	if firstHeader < 0 || secondHeader <= firstHeader || !strings.Contains(allTasks, "title: Unchanged task -> Changed second title") {
		t.Fatalf("changes were not grouped under task headings:\n%s", allTasks)
	}
	if !strings.Contains(allTasks, "description: updated") || strings.Contains(allTasks, "-Original body") {
		t.Fatalf("default diff should summarize description changes:\n%s", allTasks)
	}

	output.Reset()
	if err := diff(ws, []string{"13359"}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"Task 13359: Improve task diffs",
		"priority: medium -> high",
		"description: updated",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("diff output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "13360") || strings.Contains(text, "-Original body") {
		t.Fatalf("task selector or description diff was not applied:\n%s", text)
	}

	output.Reset()
	if err := diff(ws, []string{"--verbose", "13359"}, &output); err != nil {
		t.Fatal(err)
	}
	verbose := output.String()
	for _, expected := range []string{"description.md", "-Original body", "+Updated body", "+A new line"} {
		if !strings.Contains(verbose, expected) {
			t.Errorf("verbose diff output missing %q:\n%s", expected, verbose)
		}
	}
	if err := diff(ws, []string{"missing-task"}, &output); err == nil {
		t.Fatal("diff for an unknown task succeeded")
	}
	if err := diff(ws, []string{"13359", "13360"}, &output); err == nil {
		t.Fatal("diff accepted multiple task identifiers")
	}
}

func TestVerboseDiffShowsStagedNoteContent(t *testing.T) {
	remote := map[string]any{
		"id": "task-note-diff", "number": float64(13765), "title": "Follow up on partnership request", "type": "feature",
		"priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	ws, dir := makeSyncedTask(t, remote)
	filename := "20261028T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("Reached out to the partner.\nThey will reply next week."), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("task-note-diff")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{filename}
	if err := ws.WriteState("task-note-diff", state); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := diff(ws, []string{"--verbose"}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"Task 13765: Follow up on partnership request",
		"staged note: notes/" + filename,
		"+Reached out to the partner.",
		"+They will reply next week.",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("verbose diff output missing %q:\n%s", expected, text)
		}
	}

	output.Reset()
	if err := diff(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "notes: 1 staged") || strings.Contains(output.String(), "Reached out to the partner") {
		t.Fatalf("default diff should summarize staged notes only:\n%s", output.String())
	}
}

func TestDiffShowsManualEditsToSyncedNotes(t *testing.T) {
	timestamp := int64(1793188800000)
	remote := map[string]any{
		"id": "task-note-edit", "number": float64(13766), "title": "Edit a synced note", "type": "feature",
		"priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{},
		"notes": []any{map[string]any{
			"note": "<p>Original note body</p>", "timestamp": float64(timestamp),
			"addedBy": map[string]any{"id": "user-1", "label": "Juan"},
		}},
	}
	ws, dir := makeSyncedTask(t, remote)
	filename := noteFilename(time.UnixMilli(timestamp), "Juan", 1)
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte("Updated note body"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := diff(ws, []string{"--verbose"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"edited note: notes/" + filename, "-Original note body", "+Updated note body"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("verbose diff missing %q:\n%s", expected, output.String())
		}
	}
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if dirty, err := taskHasLocalChanges(local, workspace.State{Remote: remote}, dir); err != nil || !dirty {
		t.Fatalf("edited note dirty = %v, error = %v; want true, nil", dirty, err)
	}
}

func TestEditNoteOpensOnlyCurrentUsersSyncedNote(t *testing.T) {
	const timestamp = int64(1793188800000)
	remote := map[string]any{
		"id": "task-note-command", "number": float64(13769), "title": "Edit my note", "type": "feature",
		"priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{},
		"notes": []any{map[string]any{
			"note": "<p>Original note body</p>", "timestamp": float64(timestamp),
			"addedBy": map[string]any{"id": "user-1", "label": "Juan"},
		}},
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Edited by command" {
		t.Fatalf("editor wrote %q", data)
	}
	if !strings.Contains(output.String(), "panda diff --verbose") {
		t.Fatalf("edit command did not print review instructions: %q", output.String())
	}
}

func TestPullPreservesEditedNotesAndReportsOverlappingRemoteEdits(t *testing.T) {
	const timestamp = int64(1793188800000)
	baseline := map[string]any{
		"id": "task-note-pull", "number": float64(13768), "version": float64(1), "title": "Pull edited notes", "type": "feature",
		"priority": "medium", "complexity": "s", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{},
		"notes": []any{map[string]any{
			"note": "<p>Original note body</p>", "timestamp": float64(timestamp),
			"addedBy": map[string]any{"id": "user-1", "label": "Juan"},
		}},
	}
	remote := cloneMap(baseline)
	remote["version"] = float64(2)
	remote["status"] = "inProgress"
	remote["notes"] = []any{map[string]any{
		"note": "<p>Remote note edit</p>", "timestamp": float64(timestamp),
		"addedBy": map[string]any{"id": "user-1", "label": "Juan"},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if err := pull(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, &output); err == nil {
		t.Fatal("pull should report the overlapping note edit as a conflict")
	}
	local, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(local) != "My local note edit" {
		t.Fatalf("pull replaced the local note edit: %q", local)
	}
	if !strings.Contains(output.String(), "Conflict task 13768: notes/"+filename) {
		t.Fatalf("pull did not report the overlapping note edit:\n%s", output.String())
	}
	state, err := ws.ReadState("task-note-pull")
	if err != nil {
		t.Fatal(err)
	}
	baselineNotes, err := getRemoteNotes(state.Remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselineNotes) != 1 || baselineNotes[0].markdown != "Original note body" {
		t.Fatalf("conflicted note baseline advanced unexpectedly: %#v", baselineNotes)
	}
	if state.Remote["version"] != float64(1) {
		t.Fatalf("note-conflict baseline version = %v, want 1 so a later pull can resolve it", state.Remote["version"])
	}
	if err := clean(ws, []string{"13768"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := pull(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err != nil {
		t.Fatalf("pull after cleaning the conflicted local edit: %v", err)
	}
	local, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(local) != "Remote note edit" {
		t.Fatalf("pull after clean did not take the remote note: %q", local)
	}
}

func TestPushEditsOnlyCurrentUsersNotesWithFullMyNotesPayload(t *testing.T) {
	const ownTimestamp = int64(1793188800000)
	const ownSecondTimestamp = int64(1793188860000)
	const otherTimestamp = int64(1793188920000)
	remote := map[string]any{
		"id": "task-note-edit", "number": float64(13767), "title": "Edit a synced note", "type": "feature",
		"priority": "medium", "status": "new", "description": "", "project": map[string]any{"id": "project-1"},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{},
		"notes": []any{
			map[string]any{"note": "<p>Original note body</p>", "timestamp": float64(ownTimestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}},
			map[string]any{"note": "<p>Keep this note</p>", "timestamp": float64(ownSecondTimestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}},
			map[string]any{"note": "<p>Someone else's note</p>", "timestamp": float64(otherTimestamp), "addedBy": map[string]any{"id": "user-2", "label": "Other"}},
		},
	}
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
				t.Fatalf("myNotes count = %d, want both of the user's notes", len(request.MyNotes))
			}
			if got := request.MyNotes[0]["timestamp"]; got != float64(ownTimestamp) {
				t.Errorf("edited note timestamp = %v, want %d", got, ownTimestamp)
			}
			if !strings.Contains(stringField(request.MyNotes[0], "note"), "Updated note body") {
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
		timestamp int64
		author    string
		body      string
	}{
		{ownTimestamp, "Juan", "Original note body"},
		{ownSecondTimestamp, "Juan", "Keep this note"},
		{otherTimestamp, "Other", "Someone else's note"},
	} {
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
		t.Fatalf("successful note edit was not saved to the sync baseline: %#v", state.Remote["notes"])
	}
}

func TestDiffShowsKeyFieldsForNewTasks(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := createTask(ws, &config.Project{ID: "project-1", Name: "Alpha"}, []string{"Brand new task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("created task directories = %v, error = %v", dirs, err)
	}
	metadataPath := filepath.Join(dirs[0], "metadata.json")
	metadata, err := workspace.ReadMap(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata["priority"] = "high"
	metadata["complexity"] = "xl"
	if err := workspace.WriteMap(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := diff(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"new task",
		"type: feature",
		"priority: high",
		"complexity: xl",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("new task diff missing %q:\n%s", expected, text)
		}
	}
}

func TestDiffSummaryCountsEditedDiscardedAndNewTasks(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	editedDir := addSyncedTask(t, ws, listTask("task-edited", 41, "new", "r", "s", "Original title", "feature"))
	discardedDir := addSyncedTask(t, ws, listTask("task-discarded", 42, "inProgress", "q", "m", "Discard this", "bug"))
	for dir, updates := range map[string]map[string]any{
		editedDir:    {"title": "Updated title"},
		discardedDir: {"status": "discarded"},
	} {
		metadataPath := filepath.Join(dir, "metadata.json")
		metadata, err := workspace.ReadMap(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		for field, value := range updates {
			metadata[field] = value
		}
		if err := workspace.WriteMap(metadataPath, metadata); err != nil {
			t.Fatal(err)
		}
	}
	if err := createTask(ws, &config.Project{ID: "project-1", Name: "Alpha"}, []string{"New task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := diff(ws, []string{"--summary"}, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "Tasks: 1 edited, 1 discarded, 1 new\n"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestSummaryDiffSkipsPager(t *testing.T) {
	if shouldPageDiff([]string{"--summary"}, true, []byte("summary\n"), 80, 24) {
		t.Fatal("summary diff should not use the pager")
	}
	if shouldPageDiff([]string{"--verbose"}, true, []byte("small diff\n"), 80, 24) {
		t.Fatal("detail diff that fits the terminal should not use the pager")
	}
	if !shouldPageDiff([]string{"--verbose"}, true, []byte(strings.Repeat("line\n", 25)), 80, 24) {
		t.Fatal("detail diff longer than the terminal should use the pager")
	}
}

func TestOutputFitsScreenAccountsForWrappingAndANSI(t *testing.T) {
	if !outputFitsScreen([]byte("\x1b[1;36mheading\x1b[0m\nbody\n"), 10, 2) {
		t.Fatal("ANSI color codes should not count as screen columns")
	}
	if outputFitsScreen([]byte("12345678901\n"), 10, 1) {
		t.Fatal("wrapped output should count as multiple screen rows")
	}
}

func TestPageOutputFallsBackWhenPagerIsUnavailable(t *testing.T) {
	t.Setenv("PAGER", "panda-test-missing-pager-command")
	want := "diff content\n"
	var output bytes.Buffer
	if err := pageOutput(&output, []byte(want)); err != nil {
		t.Fatal(err)
	}
	if output.String() != want {
		t.Fatalf("fallback output = %q, want %q", output.String(), want)
	}
}

func TestEnablePagerColorsForLess(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "adds raw color option", args: []string{"less"}, want: []string{"less", "-R"}},
		{name: "preserves existing combined option", args: []string{"less", "-FRX"}, want: []string{"less", "-FRX"}},
		{name: "preserves combined raw option", args: []string{"less", "-FrX"}, want: []string{"less", "-FrX"}},
		{name: "preserves explicit raw option", args: []string{"less", "-F", "-R"}, want: []string{"less", "-F", "-R"}},
		{name: "leaves other pagers unchanged", args: []string{"more"}, want: []string{"more"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := enablePagerColors(test.args); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("enablePagerColors(%v) = %v, want %v", test.args, got, test.want)
			}
		})
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
	if err := os.WriteFile(filepath.Join(ws.TaskPath("12"), "description.md"), []byte("Has a description"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.TaskPath("13"), "description.md"), []byte(" \n\t "), 0o600); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	statusPositions := []int{
		strings.Index(text, "NEW\n"),
		strings.Index(text, "TO DO\n"),
		strings.Index(text, "IN PROGRESS\n"),
		strings.Index(text, "IN REVIEW\n"),
		strings.Index(text, "COMPLETED\n"),
		strings.Index(text, "STAGING\n"),
		strings.Index(text, "RELEASED\n"),
	}
	for i, position := range statusPositions {
		if position < 0 || (i > 0 && position <= statusPositions[i-1]) {
			t.Fatalf("workflow statuses are not grouped in the requested order: %q", text)
		}
	}
	if got := strings.Count(text, "----------------------------------------"); got != len(statusPositions)-1 {
		t.Fatalf("got %d status dividers, want %d", got, len(statusPositions)-1)
	}
	if strings.Contains(text, "Discarded") {
		t.Fatalf("discarded task appeared without an explicit status filter: %q", text)
	}
	if strings.Index(text, "New A") > strings.Index(text, "New B") {
		t.Fatalf("new tasks were not sorted by rank: %q", text)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--no-description"}, &output); err != nil {
		t.Fatal(err)
	}
	withoutDescriptions := output.String()
	if !strings.Contains(withoutDescriptions, "New A") || !strings.Contains(withoutDescriptions, "To Do") || strings.Contains(withoutDescriptions, "New B") {
		t.Fatalf("no-description filter did not keep blank descriptions and exclude populated ones: %q", withoutDescriptions)
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

func TestListShowsStoryPointsHoursAndVisibleTotals(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("new-xs", 21, "new", "a", "xs", "Extra small", "feature"),
		listTask("new-s", 22, "new", "b", "s", "Small", "feature"),
		listTask("new-unknown", 23, "new", "c", "unknown", "Unknown", "feature"),
		listTask("progress-m", 24, "inProgress", "a", "m", "Medium", "feature"),
		listTask("progress-l", 25, "inProgress", "b", "l", "Large", "feature"),
		listTask("progress-xl", 26, "inProgress", "c", "xl", "Extra large", "feature"),
		listTask("progress-xxl", 27, "inProgress", "d", "xxl", "Double extra large", "feature"),
		listTask("discarded-xs", 28, "discarded", "a", "xs", "Hidden by default", "feature"),
	}
	for _, task := range fixtures {
		addSyncedTask(t, ws, task)
	}

	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"COMPLEXITY", "SP", "HOURS", "xs", "1", "4", "xxl", "13", "52",
		"unknown", "Subtotal: 3 SP, 12 hours", "Subtotal: 29 SP, 116 hours",
		"TOTAL (visible tasks): 32 SP, 128 hours",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("list output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Hidden by default") {
		t.Fatalf("default filtering or unknown-complexity row was incorrect:\n%s", text)
	}
	unknownRowFound := false
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 7 && fields[0] == "23" {
			unknownRowFound = reflect.DeepEqual(fields[:7], []string{"23", "medium", "feature", "unknown", "-", "-", "Unknown"})
			break
		}
	}
	if !unknownRowFound {
		t.Fatalf("unrecognized complexity should show dashes in both calculated columns:\n%s", text)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--status", "new"}, &output); err != nil {
		t.Fatal(err)
	}
	filtered := output.String()
	if !strings.Contains(filtered, "Subtotal: 3 SP, 12 hours") || !strings.Contains(filtered, "TOTAL (visible tasks): 3 SP, 12 hours") || strings.Contains(filtered, "Medium") {
		t.Fatalf("filtered list totals should include only visible tasks:\n%s", filtered)
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
	dir := ws.TaskPath(workspace.DirectoryName(remote))
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
