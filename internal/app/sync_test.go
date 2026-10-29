package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
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
		case r.Method == http.MethodGet && r.URL.Path == "/data/projects/project-1":
			_, _ = fmt.Fprint(w, `{"members":[]}`)
		case r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages"):
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
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
	if !strings.Contains(output.String(), "Pulled project data: 1 task(s): 0 new, 1 updated; 0 labels, 0 packages, 0 members synchronized") || strings.Contains(output.String(), "Synchronized project") {
		t.Fatalf("pull should emit one final project-wide summary: %q", output.String())
	}
	local, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if local["title"] != "Changed with version bump" || local["version"] != float64(2) {
		t.Fatalf("versioned remote update was not applied locally: %#v", local)
	}
}

func TestSameTaskVersionStillChecksLabelAndPackageRelationships(t *testing.T) {
	baseline := map[string]any{
		"version": float64(4), "labels": []any{map[string]any{"id": "label-1", "label": "Old label"}},
		"package": map[string]any{"id": "package-1", "label": "Old package"},
	}
	remote := cloneMap(baseline)
	if !sameTaskVersion(remote, baseline) {
		t.Fatal("unchanged task version and relations should be skippable")
	}
	remote["labels"] = []any{map[string]any{"id": "label-2", "label": "New label"}}
	if sameTaskVersion(remote, baseline) {
		t.Fatal("changed labels should be pulled even without a task version bump")
	}
	remote = cloneMap(baseline)
	remote["package"] = map[string]any{"id": "package-2", "label": "New package"}
	if sameTaskVersion(remote, baseline) {
		t.Fatal("changed package should be pulled even without a task version bump")
	}
}

func TestPullProgressBarFormattingAndNonTerminalOutput(t *testing.T) {
	if got, want := formatPullProgress(0), "Pulling project data: [--------------------] 0%"; got != want {
		t.Fatalf("initial progress bar = %q, want %q", got, want)
	}
	if got, want := formatPullProgress(50), "Pulling project data: [##########----------] 50%"; got != want {
		t.Fatalf("progress bar = %q, want %q", got, want)
	}
	var output strings.Builder
	progress := newPullProgress(&output)
	progress.Update(0)
	progress.Update(50)
	progress.Finish()
	if output.Len() != 0 {
		t.Fatalf("non-terminal output included a progress bar: %q", output.String())
	}
}

func TestProjectCatalogAndMemberSyncDeduplicatesByIdentity(t *testing.T) {
	label := map[string]any{"id": "label-1", "name": "Security"}
	labels := dedupeCatalogRecords([]map[string]any{
		label,
		{"id": "label-1", "name": "Duplicate"},
		{"id": "label-2", "name": "Support"},
	})
	if len(labels) != 2 {
		t.Fatalf("catalog records were not deduplicated by ID: %#v", labels)
	}
	merged, _, _ := mergeCatalogRecords(labelCatalog, []map[string]any{label}, nil, labels)
	if len(merged) != 2 {
		t.Fatalf("catalog merge duplicated an existing project record: %#v", merged)
	}

	member := map[string]any{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "developer"}
	members := dedupeProjectMembers([]map[string]any{
		member,
		{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "developer"},
		{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "qa"},
	})
	if len(members) != 2 {
		t.Fatalf("project members were not deduplicated by user ID and role: %#v", members)
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
