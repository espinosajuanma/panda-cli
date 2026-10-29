package app

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/workspace"
)

func TestOptionEditPreservesUnspecifiedNullableAndAbsentMetadata(t *testing.T) {
	remote := map[string]any{
		"id": "task-partial", "number": float64(44), "title": "Original", "type": "feature",
		"priority": "medium", "complexity": nil, "package": nil, "labels": []any{},
		"assignees": nil, "reviewers": []any{}, "customNull": nil, "customEmpty": []any{},
		"description": "<p>Original body</p>",
	}
	ws, dir := makeSyncedTask(t, remote)
	var output bytes.Buffer
	if err := editTask(ws, []string{"44", "--priority", "high"}, strings.NewReader("input must not be prompted for"), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("option edit prompted or printed interactive output: %q", output.String())
	}
	actual, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	expected := workspace.MetadataFromRemote(remote)
	expected["priority"] = "high"
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("partial edit changed unspecified metadata:\n got: %#v\nwant: %#v", actual, expected)
	}
	if _, exists := actual["absentField"]; exists {
		t.Fatalf("partial edit introduced an absent field: %#v", actual["absentField"])
	}
}

func TestOptionEditDescriptionFileAndStdinPreserveMarkdownAndTrailingNewlines(t *testing.T) {
	description := []byte("# Heading\n\nA **multiline** body.\n\n")
	descriptionPath := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(descriptionPath, description, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", filepath.Join(t.TempDir(), "editor-must-not-run"))

	fileWS, fileDir := makeSyncedTask(t, map[string]any{
		"id": "task-file-description", "number": float64(45), "title": "File description",
		"type": "feature", "priority": "medium", "description": "<p>Old</p>",
	})
	if err := editTask(fileWS, []string{"45", "--description-file", descriptionPath}, strings.NewReader("unused stdin"), io.Discard); err != nil {
		t.Fatal(err)
	}
	fileBody, err := os.ReadFile(filepath.Join(fileDir, "description.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBody, description) {
		t.Fatalf("file description changed: got %q want %q", fileBody, description)
	}

	stdinWS, stdinDir := makeSyncedTask(t, map[string]any{
		"id": "task-stdin-description", "number": float64(46), "title": "Stdin description",
		"type": "feature", "priority": "medium", "description": "<p>Old</p>",
	})
	if err := editTask(stdinWS, []string{"46", "--description-file", "-"}, bytes.NewReader(description), io.Discard); err != nil {
		t.Fatal(err)
	}
	stdinBody, err := os.ReadFile(filepath.Join(stdinDir, "description.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdinBody, description) {
		t.Fatalf("stdin description changed: got %q want %q", stdinBody, description)
	}
}

func TestOptionEditRejectsMissingUnreadableAndEmptyDescriptionsWithoutChanges(t *testing.T) {
	remote := map[string]any{
		"id": "task-description-error", "number": float64(47), "title": "Safe", "type": "feature",
		"priority": "medium", "description": "<p>Keep this</p>",
	}
	ws, dir := makeSyncedTask(t, remote)
	missingPath := filepath.Join(t.TempDir(), "missing.md")
	emptyPath := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"47", "--description-file"},
		{"47", "--description-file", missingPath},
		{"47", "--description-file", emptyPath},
	} {
		if err := editTask(ws, args, strings.NewReader(""), io.Discard); err == nil {
			t.Errorf("editTask(%v) unexpectedly succeeded", args)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Keep this" {
		t.Fatalf("invalid description input changed the existing body: %q", body)
	}
}

func TestOptionEditExplicitClearDiffersFromOmittedFields(t *testing.T) {
	remote := map[string]any{
		"id": "task-clear", "number": float64(48), "title": "Clear fields", "type": "feature",
		"priority": "medium", "complexity": "s", "package": map[string]any{"id": "pkg-1", "label": "Backend"},
		"labels":    []any{map[string]any{"id": "label-1", "label": "Urgent"}},
		"assignees": []any{map[string]any{"id": "user-1", "label": "Developer"}}, "status": "inProgress",
		"description": "",
	}
	ws, dir := makeSyncedTask(t, remote)
	if err := writeCatalog(ws, packageCatalog, []map[string]any{{"id": "pkg-1", "title": "Backend"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, labelCatalog, []map[string]any{{"id": "label-1", "name": "Urgent"}}); err != nil {
		t.Fatal(err)
	}
	if err := editTask(ws, []string{"48", "--package", "none", "--complexity", "none", "--assignee", "none", "--remove-label", "Urgent"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"package", "complexity"} {
		if _, exists := metadata[field]; exists {
			t.Errorf("explicit clear left %s in metadata: %#v", field, metadata[field])
		}
	}
	if assignees, ok := metadata["assignees"].([]any); !ok || len(assignees) != 0 {
		t.Errorf("assignee clear = %#v, want empty array", metadata["assignees"])
	}
	if labels, ok := metadata["labels"].([]any); !ok || len(labels) != 0 {
		t.Errorf("label removal = %#v, want empty array", metadata["labels"])
	}
	if metadata["status"] != "inProgress" || metadata["priority"] != "medium" {
		t.Fatalf("omitted task fields changed: %#v", metadata)
	}
	if err := editTask(ws, []string{"48", "--label", "Urgent"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := editTask(ws, []string{"48", "--label", "none"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if labels, ok := metadata["labels"].([]any); !ok || len(labels) != 0 {
		t.Fatalf("--label none did not explicitly clear labels: %#v", metadata["labels"])
	}
	if err := editTask(ws, []string{"48", "labels", "Urgent", "--remove=Urgent"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatalf("legacy label syntax stopped working: %v", err)
	}
}

func TestCreateTaskNoninteractiveWithDescriptionDoesNotPromptOrPublish(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected API request", http.StatusInternalServerError)
	}))
	defer server.Close()

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("EDITOR", filepath.Join(t.TempDir(), "editor-must-not-run"))
	project := &config.Project{ID: "project-local", Name: "Automation", ViewID: "view-configured"}
	if err := (&config.Config{APIURL: server.URL, Project: project}).Save(); err != nil {
		t.Fatal(err)
	}
	description := []byte("Automated **Markdown** task.\n\n")
	descriptionPath := filepath.Join(t.TempDir(), "description.md")
	if err := os.WriteFile(descriptionPath, description, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run([]string{"create", "--title", "Automated task", "--type", "bug", "--description-file", descriptionPath}, strings.NewReader("must remain unused"), &output, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Title:") || strings.Contains(output.String(), "Description:") {
		t.Fatalf("option-driven create prompted: %q", output.String())
	}
	root, err := workspace.WorkspacePath(project.Name)
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(root)
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("created task dirs = %v, error=%v", dirs, err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dirs[0], "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata["title"] != "Automated task" || metadata["type"] != "bug" {
		t.Fatalf("created metadata = %#v", metadata)
	}
	body, err := os.ReadFile(filepath.Join(dirs[0], "description.md"))
	if err != nil || !bytes.Equal(body, description) {
		t.Fatalf("created description = %q, error=%v", body, err)
	}
	localID := stringField(metadata, "localId")
	if err := Run([]string{"edit", localID, "--priority", "high"}, strings.NewReader("must remain unused"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("create/edit made %d Solutions API requests; changes must remain local", requests.Load())
	}
}

func TestTaskFieldOptionsSupportRepeatableAssigneesAndLabelOperations(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, labelCatalog, []map[string]any{
		{"id": "label-feature", "name": "Feature"},
		{"id": "label-urgent", "name": "Urgent"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, packageCatalog, []map[string]any{{"id": "package-backend", "title": "Backend"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectMembers(ws, []map[string]any{
		{"user": map[string]any{"id": "user-ada", "label": "Ada"}, "role": "developer"},
		{"user": map[string]any{"id": "user-grace", "label": "Grace"}, "role": "qa"},
	}); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "p1", Name: "Alpha"}
	if err := createTask(ws, project, []string{
		"--title", "Repeatable options", "--package", "Backend",
		"--assignee", "Ada", "--assignee", "Grace",
		"--label", "Feature", "--add-label", "Urgent",
	}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("created task dirs = %v, error=%v", dirs, err)
	}
	metadataPath := filepath.Join(dirs[0], "metadata.json")
	metadata, err := workspace.ReadMap(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if relationshipID(metadata["package"]) != "package-backend" {
		t.Fatalf("noninteractive package = %#v", metadata["package"])
	}
	assignees, _ := metadata["assignees"].([]any)
	labels, _ := metadata["labels"].([]any)
	if len(assignees) != 2 || len(labels) != 2 {
		t.Fatalf("repeatable assignment options were not applied: assignees=%#v labels=%#v", assignees, labels)
	}
	if err := editTask(ws, []string{filepath.Base(dirs[0]), "--remove-label", "Feature", "--assignee", "Grace"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	assignees, _ = metadata["assignees"].([]any)
	labels, _ = metadata["labels"].([]any)
	if len(assignees) != 1 || relationshipID(assignees[0]) != "user-grace" || len(labels) != 1 || relationshipID(labels[0]) != "label-urgent" {
		t.Fatalf("repeatable add/remove options were not applied: assignees=%#v labels=%#v", assignees, labels)
	}
}

func TestLinkUsesConfiguredProjectViewAndVerifiedTaskMetadataID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := &config.Project{ID: "project-1", Name: "Linked", ViewID: "configured-view-1"}
	if err := (&config.Config{Project: project}).Save(); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.WorkspacePath(project.Name)
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	remote := map[string]any{
		"id": "verified-task-id", "number": float64(12345), "title": "Link target",
		"entity": map[string]any{"id": "wrong-entity-id"}, "type": "feature", "priority": "medium", "description": "",
	}
	addSyncedTask(t, ws, remote)
	var output bytes.Buffer
	if err := Run([]string{"link", "12345"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	want := "[#12345](https://solutions.slingrs.io/prod/runtime/#views/configured-view-1/readOnly/verified-task-id)\n"
	if output.String() != want {
		t.Fatalf("task link = %q, want %q", output.String(), want)
	}
}

func TestCanonicalTaskLinkRejectsMissingIdentifiers(t *testing.T) {
	if _, err := canonicalTaskLink("", float64(1), "verified-id"); err == nil || !strings.Contains(err.Error(), "project view ID") {
		t.Fatalf("missing view ID error = %v", err)
	}
	if _, err := canonicalTaskLink("view-id", float64(1), ""); err == nil || !strings.Contains(err.Error(), "task ID") {
		t.Fatalf("missing task ID error = %v", err)
	}
	if _, err := canonicalTaskLink("view-id", nil, "task-id"); err == nil || !strings.Contains(err.Error(), "task number") {
		t.Fatalf("missing task number error = %v", err)
	}
	if _, err := canonicalTaskLink("view-id", "[malicious]", "task-id"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid task number error = %v", err)
	}

	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := createTask(ws, &config.Project{ID: "p1", Name: "Local"}, []string{"Unlinked local task"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("local task directories = %v, error=%v", dirs, err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dirs[0], "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	localID := stringField(metadata, "localId")
	if err := linkTask(ws, []string{localID}, "view-id", io.Discard); err == nil || !strings.Contains(err.Error(), "no verified Solutions task ID") {
		t.Fatalf("local task link error = %v", err)
	}
}

func TestNoninteractiveTaskOptionErrorsAreExplicit(t *testing.T) {
	ws, _ := makeSyncedTask(t, map[string]any{
		"id": "task-option-errors", "number": float64(49), "title": "Keep", "type": "feature",
		"priority": "medium", "description": "",
	})
	for _, args := range [][]string{
		{"49", "--unknown-field", "value"},
		{"49", "--priority"},
		{"49", "--priority", "impossible"},
	} {
		if err := editTask(ws, args, strings.NewReader("unused"), io.Discard); err == nil {
			t.Errorf("editTask(%v) unexpectedly succeeded", args)
		}
	}
	if err := createTask(ws, &config.Project{ID: "project-1", Name: "Alpha"}, []string{"--type", "bug"}, strings.NewReader("unused"), io.Discard); err == nil || !strings.Contains(err.Error(), "requires --title") {
		t.Fatalf("incomplete noninteractive create error = %v", err)
	}
}

func TestTaskDescriptionRejectsInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTaskDescription(path, nil); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
}

func TestProjectViewConfigCommandPersistsAndDisplaysValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{Project: &config.Project{ID: "p1", Name: "Alpha", ViewID: config.DefaultProjectViewID}}
	var output bytes.Buffer
	if err := configCommand(nil, cfg, nil, []string{"set", "project-view", "custom-view"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := configCommand(nil, loaded, nil, []string{"get", "project-view"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "custom-view\n" {
		t.Fatalf("configured view output = %q", output.String())
	}
}

func TestTaskOptionCreateUsesNormalLocalWorkspacePersistence(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := createTask(ws, &config.Project{ID: "p1", Name: "Alpha"}, []string{"--title=Automated", "--description-file", "-"}, strings.NewReader("body\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("task directories = %v, error=%v", dirs, err)
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "metadata.json")); err != nil {
		t.Fatal(fmt.Errorf("metadata was not written through workspace persistence: %w", err))
	}
}
