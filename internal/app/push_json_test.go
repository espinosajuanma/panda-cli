package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestParsePushOptionsAcceptsJSONAmongSelectors(t *testing.T) {
	options, err := parsePushOptions([]string{"draft-1", "--json", "42"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.json || len(options.selectors) != 2 || options.selectors[0] != "draft-1" || options.selectors[1] != "42" {
		t.Fatalf("push options = %#v", options)
	}
	if _, err := parsePushOptions([]string{"--unknown"}); err == nil {
		t.Fatal("unknown push option was accepted")
	}
}

func TestPushJSONReportsCreatedTask(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := createTask(ws, project, []string{"JSON created task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/data/dev.tasks" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var created map[string]any
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Error(err)
			return
		}
		created["id"], created["number"], created["version"], created["notes"] = "remote-created", float64(77), float64(1), []any{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(created)
	}))
	defer server.Close()

	var output strings.Builder
	err := pushSelectedWithOptions(context.Background(), solutions.NewClient(server.URL, "token"), ws, project, pushOptions{json: true}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var report pushReport
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatalf("invalid JSON push report %q: %v", output.String(), err)
	}
	if report.SchemaVersion != 1 || len(report.Tasks) != 1 {
		t.Fatalf("JSON push report = %#v", report)
	}
	result := report.Tasks[0]
	if result.Selector != "draft-1" || result.Outcome != "created" || !result.RemoteCreated || result.ID != "remote-created" || fmt.Sprint(result.Number) != "77" || result.Title != "JSON created task" {
		t.Fatalf("created task result = %#v", result)
	}
	if len(result.Actions) != 1 || result.Actions[0] != "create" || report.Summary != (pushSummary{Pushed: 1, Created: 1}) {
		t.Fatalf("created task actions/summary = %#v / %#v", result.Actions, report.Summary)
	}
	if strings.Contains(output.String(), "Created task") || strings.Contains(output.String(), "Pushed ") {
		t.Fatalf("human-readable output leaked into JSON mode: %q", output.String())
	}
}

func TestPushJSONReportsUpdatedTask(t *testing.T) {
	remote := map[string]any{
		"id": "task-json-update", "number": float64(78), "title": "Original title", "type": "feature", "priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-json-update":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-json-update":
			if err := json.NewDecoder(r.Body).Decode(&remote); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	cleanRemote := map[string]any{
		"id": "task-json-clean", "number": float64(80), "title": "Unchanged task", "type": "feature", "priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	addSyncedTask(t, ws, cleanRemote)
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["title"] = "Updated title"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	err = pushSelectedWithOptions(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, pushOptions{json: true}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var report pushReport
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatalf("invalid JSON push report %q: %v", output.String(), err)
	}
	if len(report.Tasks) != 2 {
		t.Fatalf("updated task result = %#v", report.Tasks)
	}
	updated, skipped := report.Tasks[0], report.Tasks[1]
	if updated.Outcome != "pushed" || len(updated.Actions) != 1 || updated.Actions[0] != "fields" || skipped.Outcome != "skipped" {
		t.Fatalf("updated/skipped task results = %#v / %#v", updated, skipped)
	}
	if report.Summary != (pushSummary{Pushed: 1, Updated: 1, Skipped: 1}) {
		t.Fatalf("updated task summary = %#v", report.Summary)
	}
}

func TestPushJSONEmitsConflictReportBeforeReturningError(t *testing.T) {
	remote := map[string]any{
		"id": "task-json-conflict", "number": float64(79), "title": "Remote title", "type": "feature", "priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/dev.tasks/task-json-conflict" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(remote)
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, map[string]any{
		"id": "task-json-conflict", "number": float64(79), "title": "Original title", "type": "feature", "priority": "medium", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	})
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata["title"] = "Local title"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	err = pushSelectedWithOptions(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, pushOptions{json: true}, &output)
	if err == nil || !strings.Contains(err.Error(), "push had 1 failure") {
		t.Fatalf("push error = %v, want one reported failure", err)
	}
	var report pushReport
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatalf("invalid JSON conflict report %q: %v", output.String(), err)
	}
	if len(report.Tasks) != 1 || report.Tasks[0].Outcome != "conflict" || report.Summary.Failed != 1 || len(report.Errors) != 1 {
		t.Fatalf("conflict report = %#v", report)
	}
	if strings.Contains(output.String(), "Error:") || strings.Contains(output.String(), "Pushed ") {
		t.Fatalf("human-readable output leaked into JSON mode: %q", output.String())
	}
}
