package sync

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

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

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
	if err := Pull(context.Background(), client, ws, project, &strings.Builder{}); err != nil {
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
	if err := Push(context.Background(), client, ws, project, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if remote["title"] != "Locally edited" || remote["status"] != "inProgress" {
		t.Fatalf("independent updates were not merged: %#v", remote)
	}
	if !strings.Contains(jsonutil.StringField(remote, "description"), "Updated ") || putCount != 1 {
		t.Fatalf("local description was not pushed: puts=%d, description=%q", putCount, jsonutil.StringField(remote, "description"))
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
	if err := Pull(context.Background(), client, ws, project, &output); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("51")

	remote["title"] = "Changed without version bump"
	output.Reset()
	if err := Pull(context.Background(), client, ws, project, &output); err != nil {
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
	if err := Pull(context.Background(), client, ws, project, &output); err != nil {
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
		"package": map[string]any{"id": "package-1", "label": "Old package"}, "files": []any{},
	}
	remote := jsonutil.CloneMap(baseline)
	if !SameTaskVersion(remote, baseline) {
		t.Fatal("unchanged task version and relations should be skippable")
	}
	remote["labels"] = []any{map[string]any{"id": "label-2", "label": "New label"}}
	if SameTaskVersion(remote, baseline) {
		t.Fatal("changed labels should be pulled even without a task version bump")
	}
	remote = jsonutil.CloneMap(baseline)
	remote["package"] = map[string]any{"id": "package-2", "label": "New package"}
	if SameTaskVersion(remote, baseline) {
		t.Fatal("changed package should be pulled even without a task version bump")
	}
	remote = jsonutil.CloneMap(baseline)
	remote["files"] = []any{map[string]any{"file": map[string]any{"id": "file-1"}}}
	if SameTaskVersion(remote, baseline) {
		t.Fatal("changed files should be pulled even without a task version bump")
	}
}

func TestPullProgressBarFormattingAndNonTerminalOutput(t *testing.T) {
	if got, want := FormatPullProgress(0), "Pulling project data: [--------------------] 0%"; got != want {
		t.Fatalf("initial progress bar = %q, want %q", got, want)
	}
	if got, want := FormatPullProgress(50), "Pulling project data: [##########----------] 50%"; got != want {
		t.Fatalf("progress bar = %q, want %q", got, want)
	}
	var output strings.Builder
	progress := NewPullProgress(&output)
	progress.Update(0)
	progress.Update(50)
	progress.Finish()
	if output.Len() != 0 {
		t.Fatalf("non-terminal output included a progress bar: %q", output.String())
	}
}

func TestProjectCatalogAndMemberSyncDeduplicatesByIdentity(t *testing.T) {
	label := map[string]any{"id": "label-1", "name": "Security"}
	labels := DedupeCatalogRecords([]map[string]any{
		label,
		{"id": "label-1", "name": "Duplicate"},
		{"id": "label-2", "name": "Support"},
	})
	if len(labels) != 2 {
		t.Fatalf("catalog records were not deduplicated by ID: %#v", labels)
	}
	merged, _, _ := catdomain.MergeRecords(catdomain.Labels, []map[string]any{label}, nil, labels)
	if len(merged) != 2 {
		t.Fatalf("catalog merge duplicated an existing project record: %#v", merged)
	}

	member := map[string]any{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "developer"}
	members := DedupeProjectMembers([]map[string]any{
		member,
		{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "developer"},
		{"user": map[string]any{"id": "user-1", "label": "Ada"}, "role": "qa"},
	})
	if len(members) != 2 {
		t.Fatalf("project members were not deduplicated by user ID and role: %#v", members)
	}
}
