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

func marshalForTestSlice(records []map[string]any) string {
	data, _ := json.Marshal(records)
	return string(data)
}

func TestPushCreatesCatalogItemsMapsTaskAssignmentsAndDeletesAfterTaskUpdate(t *testing.T) {
	labels, packages := []map[string]any{}, []map[string]any{}
	var remoteTask map[string]any
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.labels":
			_, _ = fmt.Fprintf(w, `{"total":%d,"items":%s}`, len(labels), marshalForTestSlice(labels))
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.packages":
			_, _ = fmt.Fprintf(w, `{"total":%d,"items":%s}`, len(packages), marshalForTestSlice(packages))
		case r.Method == http.MethodPost && r.URL.Path == "/data/dev.labels":
			var record map[string]any
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				t.Fatal(err)
			}
			record["id"] = fmt.Sprintf("label-%d", len(labels)+1)
			record["label"] = stringField(record, "name")
			labels = append(labels, record)
			_ = json.NewEncoder(w).Encode(record)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/data/dev.labels/"):
			var record map[string]any
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				t.Fatal(err)
			}
			for index, existing := range labels {
				if stringField(existing, "id") == filepath.Base(r.URL.Path) {
					labels[index] = record
					break
				}
			}
			_ = json.NewEncoder(w).Encode(record)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/data/dev.labels/"):
			events = append(events, "delete-label")
			id := filepath.Base(r.URL.Path)
			for index, existing := range labels {
				if stringField(existing, "id") == id {
					labels = append(labels[:index], labels[index+1:]...)
					break
				}
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/data/dev.packages":
			var record map[string]any
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				t.Fatal(err)
			}
			record["id"] = fmt.Sprintf("package-%d", len(packages)+1)
			record["label"] = stringField(record, "title")
			if !strings.Contains(stringField(record, "description"), "<p>") {
				t.Errorf("package description was not converted to HTML on create: %#v", record)
			}
			packages = append(packages, record)
			_ = json.NewEncoder(w).Encode(record)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/data/dev.packages/"):
			var record map[string]any
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stringField(record, "description"), "<p>") {
				t.Errorf("package description was not converted to HTML on update: %#v", record)
			}
			for index, existing := range packages {
				if stringField(existing, "id") == filepath.Base(r.URL.Path) {
					packages[index] = record
					break
				}
			}
			_ = json.NewEncoder(w).Encode(record)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/data/dev.packages/"):
			events = append(events, "delete-package")
			id := filepath.Base(r.URL.Path)
			for index, existing := range packages {
				if stringField(existing, "id") == id {
					packages = append(packages[:index], packages[index+1:]...)
					break
				}
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/data/dev.tasks":
			if err := json.NewDecoder(r.Body).Decode(&remoteTask); err != nil {
				t.Fatal(err)
			}
			taskLabels, _ := remoteTask["labels"].([]any)
			if len(taskLabels) != 1 || stringField(taskLabels[0].(map[string]any), "id") != "label-1" {
				t.Errorf("task label was not mapped to its remote catalog ID: %#v", remoteTask["labels"])
			}
			if got := relationshipID(remoteTask["package"]); got != "package-1" {
				t.Errorf("task package was not mapped to its remote catalog ID: %q", got)
			}
			remoteTask["id"], remoteTask["number"], remoteTask["status"], remoteTask["notes"] = "task-1", float64(42), "new", []any{}
			_ = json.NewEncoder(w).Encode(remoteTask)
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			_ = json.NewEncoder(w).Encode(remoteTask)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1":
			events = append(events, "task-update")
			if err := json.NewDecoder(r.Body).Decode(&remoteTask); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(remoteTask)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	var output strings.Builder
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"create", "Urgent"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"create", "Migration", "Initial package"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if err := createTask(ws, project, []string{"New task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("created task directories = %v, error=%v", dirs, err)
	}
	metadataPath := filepath.Join(dirs[0], "metadata.json")
	metadata, err := workspace.ReadMap(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	localLabels, _ := readCatalog(ws, labelCatalog)
	localPackages, _ := readCatalog(ws, packageCatalog)
	metadata["labels"] = []any{map[string]any{"id": stringField(localLabels[0], "localId"), "label": "Urgent"}}
	metadata["package"] = map[string]any{"id": stringField(localPackages[0], "localId"), "label": "Migration"}
	if err := workspace.WriteMap(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}
	client := solutions.NewClient(server.URL, "token")
	if err := push(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || len(packages) != 1 || stringField(remoteTask, "id") != "task-1" {
		t.Fatalf("catalog/task creates missing: labels=%#v packages=%#v task=%#v", labels, packages, remoteTask)
	}
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"edit", "Urgent"}, strings.NewReader("Critical\nNew label description\n"), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"edit", "Migration"}, strings.NewReader("Platform migration\nNew package description\n"), &output); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	if stringField(labels[0], "name") != "Critical" || stringField(packages[0], "title") != "Platform migration" {
		t.Fatalf("catalog edits were not pushed: labels=%#v packages=%#v", labels, packages)
	}
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"delete", "Critical"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"delete", "Platform migration"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	events = nil
	if err := push(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(remoteTask["labels"].([]any)) != 0 || hasPackageAssignment(remoteTask["package"]) {
		t.Fatalf("task assignments remain after catalog deletion: labels=%#v package=%#v", remoteTask["labels"], remoteTask["package"])
	}
	if len(labels) != 0 || len(packages) != 0 {
		t.Fatalf("catalog items remain after delete push: labels=%#v packages=%#v", labels, packages)
	}
	if len(events) != 3 || events[0] != "task-update" || events[1] != "delete-label" || events[2] != "delete-package" {
		t.Fatalf("catalog deletes must follow task assignment updates, got events %v", events)
	}
}
