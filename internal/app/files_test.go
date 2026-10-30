package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestFilesUploadStagesSourcePathWithoutUploading(t *testing.T) {
	remote := fileTestTask()
	ws, _ := makeSyncedTask(t, remote)
	source := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(source, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := filesCommand(ws, []string{"upload", "42", source, "--description", "Test file"}, &solutions.Client{}, &output); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.StagedFiles) != 1 || state.StagedFiles[0].Path != source || state.StagedFiles[0].Name != "notes.txt" || state.StagedFiles[0].Description != "Test file" {
		t.Fatalf("unexpected staged file state: %#v", state.StagedFiles)
	}
	if !strings.Contains(output.String(), "Staged file notes.txt") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestPushUploadsAndAttachesStagedFiles(t *testing.T) {
	remote := fileTestTask()
	source := filepath.Join(t.TempDir(), "brief.txt")
	if err := os.WriteFile(source, []byte("file for task"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, _ := makeSyncedTask(t, remote)
	state, err := ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedFiles = []workspace.StagedFile{{Path: source, Name: "brief.txt", Description: "Requirements"}}
	if err := ws.WriteState("task-files", state); err != nil {
		t.Fatal(err)
	}
	var uploadSeen, attachSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-files":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPost && r.URL.Path == "/files":
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/form-data" {
				t.Errorf("unexpected upload content type: %q, err=%v", r.Header.Get("Content-Type"), err)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			part, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Error(err)
				return
			}
			if part.FileName() != "brief.txt" || string(data) != "file for task" {
				t.Errorf("uploaded filename/content = %q/%q", part.FileName(), data)
			}
			uploadSeen = true
			_, _ = io.WriteString(w, `{"fileId":"uploaded-1","fileLink":"/files/uploaded-1"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-files/attachFile":
			var body map[string][]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			items := body["files"]
			if len(items) != 1 || relationshipID(items[0]["file"]) != "uploaded-1" || stringField(items[0], "description") != "Requirements" {
				t.Errorf("unexpected attach body: %#v", body)
			}
			remote["files"] = []any{map[string]any{
				"file":        map[string]any{"id": "uploaded-1", "name": "brief.txt (13B)"},
				"description": "Requirements",
			}}
			attachSeen = true
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var output strings.Builder
	client := solutions.NewClient(server.URL, "token")
	if err := push(context.Background(), client, ws, &config.Project{ID: "project-1", Name: "Alpha"}, &output); err != nil {
		t.Fatal(err)
	}
	if !uploadSeen || !attachSeen {
		t.Fatalf("upload/attach requests seen = %v/%v", uploadSeen, attachSeen)
	}
	local, err := workspace.ReadMap(filepath.Join(ws.TaskPath("42"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	files, ok := local["files"].([]any)
	if !ok || len(files) != 1 || taskFileReferences(local)[0].ID != "uploaded-1" {
		t.Fatalf("local task does not contain attached file reference: %#v", local["files"])
	}
	state, err = ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.StagedFiles) != 0 || taskFileReferences(state.Remote)[0].ID != "uploaded-1" {
		t.Fatalf("staged state or remote baseline not advanced: %#v", state)
	}
}

func TestFilesDownloadWritesToRequestedPath(t *testing.T) {
	remote := fileTestTask()
	remote["files"] = []any{map[string]any{
		"file":        map[string]any{"id": "file-1", "name": "report.txt (15B)"},
		"description": "Build report",
	}}
	ws, _ := makeSyncedTask(t, remote)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/dev.tasks/task-files/files/file-1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
		_, _ = io.WriteString(w, "report content")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "download.txt")
	var output strings.Builder
	client := solutions.NewClient(server.URL, "token")
	if err := filesCommand(ws, []string{"download", "42", "file-1", "--output", destination}, client, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "report content" || !strings.Contains(output.String(), destination) {
		t.Fatalf("download content=%q output=%q", data, output.String())
	}
}

func TestFilesListShowsAttachedAndStagedReferences(t *testing.T) {
	remote := fileTestTask()
	remote["files"] = []any{map[string]any{
		"file":        map[string]any{"id": "file-1", "name": "report.txt"},
		"description": "Build report",
	}}
	ws, dir := makeSyncedTask(t, remote)
	source := filepath.Join(t.TempDir(), "pending.pdf")
	if err := os.WriteFile(source, []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedFiles = []workspace.StagedFile{{Path: source, Name: "pending.pdf"}}
	if err := ws.WriteState("task-files", state); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := renderTaskFiles(ws, dir, workspace.MetadataFromRemote(remote), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"report.txt", "file-1", "Build report", "pending.pdf", "[staged]"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("file listing missing %q: %s", expected, output.String())
		}
	}
}

func TestCleanClearsStagedFileUploads(t *testing.T) {
	remote := fileTestTask()
	ws, _ := makeSyncedTask(t, remote)
	state, err := ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedFiles = []workspace.StagedFile{{Path: "/tmp/keep-until-push.txt", Name: "keep-until-push.txt"}}
	if err := ws.WriteState("task-files", state); err != nil {
		t.Fatal(err)
	}
	if err := clean(ws, []string{"42"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	state, err = ws.ReadState("task-files")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.StagedFiles) != 0 {
		t.Fatalf("clean left staged file uploads: %#v", state.StagedFiles)
	}
}

func fileTestTask() map[string]any {
	return map[string]any{
		"id": "task-files", "number": float64(42), "version": float64(1),
		"title": "File task", "type": "feature", "priority": "medium", "status": "new",
		"project":     map[string]any{"id": "project-1", "label": "Alpha"},
		"description": "<p>Task body</p>", "files": []any{}, "notes": []any{},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{},
	}
}

func TestFilesDownloadTemporaryPathIsPrintedAndReadable(t *testing.T) {
	remote := fileTestTask()
	remote["files"] = []any{map[string]any{"file": map[string]any{"id": "file-1", "name": "artifact.bin"}}}
	ws, _ := makeSyncedTask(t, remote)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "artifact bytes")
	}))
	defer server.Close()

	var output strings.Builder
	client := solutions.NewClient(server.URL, "token")
	if err := filesCommand(ws, []string{"download", "42", "1"}, client, &output); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(output.String())
	path := strings.TrimPrefix(line, "Downloaded artifact.bin to ")
	if path == line || !filepath.IsAbs(path) {
		t.Fatalf("unexpected temp download path: %q", line)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "artifact bytes" {
		t.Fatalf("temp file content=%q, err=%v", data, err)
	}
}
