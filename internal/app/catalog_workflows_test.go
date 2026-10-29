package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

func TestInteractiveEditValidatesEnumsAndWritesMetadata(t *testing.T) {
	ws := workspace.New(t.TempDir())
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
	if err := editTask(ws, []string{"14"}, strings.NewReader("Changed title\n\nhigh\n-\nn\n\n"), io.Discard); err != nil {
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
	dir := filepath.Join(t.TempDir(), "14")
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
	actual, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if actual["title"] != "Original" {
		t.Fatalf("partial invalid edit was saved: %#v", actual)
	}
}

func TestInteractiveLabelPickerAddsAndRemovesAPIRelationships(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, labelCatalog, []map[string]any{{"id": "l1", "name": "Feature A"}, {"id": "l2", "name": "Feature B"}}); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "labels": []any{}}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := editLabels(ws, dir, metadata, []string{"Feature A", "Feature B"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ := metadata["labels"].([]any)
	if len(labels) != 2 || stringField(labels[0].(map[string]any), "id") != "l1" || stringField(labels[1].(map[string]any), "id") != "l2" {
		t.Fatalf("label relationship not saved: %#v", metadata["labels"])
	}
	if err := editLabels(ws, dir, metadata, []string{"--remove=Feature A"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ = metadata["labels"].([]any)
	if len(labels) != 1 || stringField(labels[0].(map[string]any), "id") != "l2" {
		t.Fatalf("label relationship not removed: %#v", labels)
	}
	if err := editLabels(ws, dir, metadata, []string{"none"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _ = metadata["labels"].([]any)
	if len(labels) != 0 {
		t.Fatalf("none selector did not clear labels: %#v", labels)
	}
}

func TestPackageSelectorSupportsOneOrNone(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
	metadata := map[string]any{"id": "task-1", "type": "feature"}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, packageCatalog, []map[string]any{{"id": "p1", "title": "Epic A"}, {"id": "p2", "title": "Epic B"}}); err != nil {
		t.Fatal(err)
	}
	if err := editTaskPackage(ws, dir, metadata, []string{"Epic B"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := relationshipID(metadata["package"]); got != "p2" {
		t.Fatalf("selected package ID = %q, want p2", got)
	}
	if err := editTaskPackage(ws, dir, metadata, []string{"none"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if hasPackageAssignment(metadata["package"]) {
		t.Fatalf("none selector did not clear package: %#v", metadata["package"])
	}
}

func TestRoleAssignmentAndCreateEditAssigneesSupportMultipleMembers(t *testing.T) {
	remote := map[string]any{"id": "task-assign", "number": float64(14), "title": "Assignable", "type": "feature", "priority": "medium", "status": "new", "description": "", "assignees": []any{}}
	ws, _ := makeSyncedTask(t, remote)
	members := []map[string]any{
		{"user": map[string]any{"id": "user-lead", "label": "Ada Lovelace", "email": "ada@example.com"}, "role": "teamLead"},
		{"user": map[string]any{"id": "user-dev", "label": "Grace Hopper", "email": "grace@example.com"}, "role": "developer"},
		{"user": map[string]any{"id": "user-qa", "label": "Linus Torvalds", "email": "linus@example.com"}, "role": "qa"},
		{"user": map[string]any{"id": "user-ux", "label": "Ursula UX", "email": "ursula@example.com"}, "role": "ux"},
		{"user": map[string]any{"id": "user-support", "label": "Sam Support", "email": "sam@example.com"}, "role": "support"},
	}
	if err := writeProjectMembers(ws, members); err != nil {
		t.Fatal(err)
	}
	syncedMembers, err := projectMembers(ws)
	if err != nil || len(syncedMembers) != 5 {
		t.Fatalf("non-assignee roles should remain synced: members=%#v error=%v", syncedMembers, err)
	}
	if eligible := eligibleProjectMembers(syncedMembers); len(eligible) != 3 {
		t.Fatalf("assignee eligibility included unsupported roles: %#v", eligible)
	}
	if err := assignTask(ws, []string{"14", "teamLead,developer"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	dir, metadata, err := ws.FindTask("14")
	if err != nil {
		t.Fatal(err)
	}
	assignees, _ := metadata["assignees"].([]any)
	if len(assignees) != 2 || relationshipID(assignees[0]) != "user-lead" || relationshipID(assignees[1]) != "user-dev" {
		t.Fatalf("role assignment did not select all matching members: %#v", metadata["assignees"])
	}
	if err := editTask(ws, []string{"14", "assignees", "Ada Lovelace", "Linus Torvalds"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := editTask(ws, []string{"14", "assignees", "Ursula UX"}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "not eligible for assignment") {
		t.Fatalf("an ineligible member was accepted as an assignee: %v", err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	assignees, _ = metadata["assignees"].([]any)
	if len(assignees) != 2 || relationshipID(assignees[0]) != "user-lead" || relationshipID(assignees[1]) != "user-qa" {
		t.Fatalf("edit assignees did not save explicit members: %#v", metadata["assignees"])
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := createTask(ws, project, []string{"New", "task", "--assignee", "Ada Lovelace,Grace Hopper"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	for _, taskDir := range dirs {
		candidate, readErr := workspace.ReadMap(filepath.Join(taskDir, "metadata.json"))
		if readErr == nil && stringField(candidate, "title") == "New task" {
			created = candidate
			break
		}
	}
	if created == nil {
		t.Fatal("create did not save task metadata")
	}
	assignees, _ = created["assignees"].([]any)
	if len(assignees) != 2 {
		t.Fatalf("create did not save multiple assignees: %#v", created["assignees"])
	}
}

func TestInteractiveLabelPickerSearchesPrefixes(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, labelCatalog, []map[string]any{{"id": "l1", "name": "Feature A"}, {"id": "l2", "name": "Fix"}}); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "labels": []any{}}
	var output strings.Builder
	if err := editLabels(ws, dir, metadata, nil, strings.NewReader("fe\n1\n\n"), &output); err != nil {
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

func TestProjectLabelAndPackageCommandsCreateEditAndDeleteLocally(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	var output strings.Builder
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"create", "Security", "Review label"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"create", "Migration", "Large package"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	labels, err := readCatalog(ws, labelCatalog)
	if err != nil || len(labels) != 1 || stringField(labels[0], "localId") == "" {
		t.Fatalf("local labels = %#v, error = %v", labels, err)
	}
	packages, err := readCatalog(ws, packageCatalog)
	if err != nil || len(packages) != 1 || stringField(packages[0], "localId") == "" {
		t.Fatalf("local packages = %#v, error = %v", packages, err)
	}
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"edit", "Security"}, strings.NewReader("Critical\nUpdated label\n"), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"edit", "Migration"}, strings.NewReader("Platform migration\nUpdated package\n"), &output); err != nil {
		t.Fatal(err)
	}
	labels, _ = readCatalog(ws, labelCatalog)
	packages, _ = readCatalog(ws, packageCatalog)
	if stringField(labels[0], "name") != "Critical" || stringField(labels[0], "description") != "Updated label" {
		t.Fatalf("label edit not saved locally: %#v", labels[0])
	}
	if stringField(packages[0], "title") != "Platform migration" || stringField(packages[0], "description") != "Updated package" {
		t.Fatalf("package edit not saved locally: %#v", packages[0])
	}
	taskDir := ws.TaskPath("local-test")
	if err := os.MkdirAll(taskDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": "task-1", "labels": []any{map[string]any{"id": stringField(labels[0], "localId"), "label": "Critical"}}, "package": map[string]any{"id": stringField(packages[0], "localId"), "label": "Platform migration"}}
	if err := workspace.WriteMap(filepath.Join(taskDir, "metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"delete", "Critical"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"delete", "Platform migration"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(taskDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if labels, _ := metadata["labels"].([]any); len(labels) != 0 {
		t.Fatalf("deleted label remained assigned to task: %#v", labels)
	}
	if _, exists := metadata["package"]; exists {
		t.Fatalf("deleted package remained assigned to task: %#v", metadata["package"])
	}
}

func TestCleanWholeProjectDiscardsStagedCatalogChanges(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := projectCatalogCommand(ws, project, labelCatalog, []string{"create", "Staged label"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := projectCatalogCommand(ws, project, packageCatalog, []string{"create", "Staged package"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := clean(ws, []string{"--yes"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	labels, err := readCatalog(ws, labelCatalog)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := readCatalog(ws, packageCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 0 || len(packages) != 0 {
		t.Fatalf("clean left staged catalog records: labels=%#v packages=%#v", labels, packages)
	}
}

func TestPullProjectCatalogsMergesIndependentEditsAndReportsConflicts(t *testing.T) {
	labels := []map[string]any{{"id": "label-1", "name": "Security", "description": "Original"}}
	packages := []map[string]any{{"id": "package-1", "title": "Migration", "description": "<p>Original package</p>"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/data/dev.labels":
			_, _ = fmt.Fprintf(w, `{"total":%d,"items":%s}`, len(labels), marshalForTestSlice(labels))
		case "/data/dev.packages":
			_, _ = fmt.Fprintf(w, `{"total":%d,"items":%s}`, len(packages), marshalForTestSlice(packages))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	client := solutions.NewClient(server.URL, "token")
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if conflicts, err := pullProjectCatalogs(context.Background(), client, ws, project, io.Discard); err != nil || conflicts != 0 {
		t.Fatalf("initial catalog pull conflicts=%d, error=%v", conflicts, err)
	}
	localLabels, err := readCatalog(ws, labelCatalog)
	if err != nil {
		t.Fatal(err)
	}
	localPackages, err := readCatalog(ws, packageCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stringField(localPackages[0], "description"), "<p>") || !strings.Contains(stringField(localPackages[0], "description"), "Original package") {
		t.Fatalf("package description was not converted to Markdown locally: %#v", localPackages[0])
	}
	localLabels[0]["description"] = "Local description"
	if err := writeCatalog(ws, labelCatalog, localLabels); err != nil {
		t.Fatal(err)
	}
	labels[0]["name"] = "Security reviews"
	var output strings.Builder
	conflicts, err := pullProjectCatalogs(context.Background(), client, ws, project, &output)
	if err != nil || conflicts != 0 {
		t.Fatalf("independent catalog pull conflicts=%d, error=%v, output=%s", conflicts, err, output.String())
	}
	localLabels, _ = readCatalog(ws, labelCatalog)
	if stringField(localLabels[0], "name") != "Security reviews" || stringField(localLabels[0], "description") != "Local description" {
		t.Fatalf("independent catalog changes were not merged: %#v", localLabels[0])
	}
	localLabels[0]["name"] = "Local name"
	if err := writeCatalog(ws, labelCatalog, localLabels); err != nil {
		t.Fatal(err)
	}
	labels[0]["name"] = "Remote name"
	output.Reset()
	conflicts, err = pullProjectCatalogs(context.Background(), client, ws, project, &output)
	if err != nil || conflicts != 1 || !strings.Contains(output.String(), "Conflict project labels") {
		t.Fatalf("overlapping catalog pull conflicts=%d, error=%v, output=%s", conflicts, err, output.String())
	}
	localLabels, _ = readCatalog(ws, labelCatalog)
	if stringField(localLabels[0], "name") != "Local name" {
		t.Fatalf("pull overwrote the local label conflict: %#v", localLabels[0])
	}
}

func TestPullMergesTaskLabelAndPackageAssignments(t *testing.T) {
	baseline := map[string]any{
		"id": "task-relations", "number": float64(61), "version": float64(1), "title": "Task relations", "type": "feature",
		"priority": "medium", "complexity": "s", "status": "new", "description": "",
		"project": map[string]any{"id": "project-1"}, "labels": []any{map[string]any{"id": "label-1", "label": "Old label"}},
		"package": map[string]any{"id": "package-1", "label": "Old package"}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	remote := cloneMap(baseline)
	remote["version"] = float64(2)
	remote["labels"] = []any{map[string]any{"id": "label-2", "label": "Remote label"}}
	remote["package"] = map[string]any{"id": "package-2", "label": "Remote package"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/data/projects/project-1":
			_, _ = fmt.Fprint(w, `{"members":[{"user":{"id":"user-lead","label":"Ada Lovelace","email":"ada@example.com"},"role":"teamLead"},{"user":{"id":"user-dev","label":"Grace Hopper","email":"grace@example.com"},"role":"developer"}]}`)
		case "/data/dev.labels", "/data/dev.packages":
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
		case "/data/dev.tasks":
			_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, baseline)
	client := solutions.NewClient(server.URL, "token")
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	projectTeam, err := projectMembers(ws)
	if err != nil || len(projectTeam) != 2 || stringField(projectTeam[0], "role") != "teamLead" {
		t.Fatalf("project members were not synced with roles: %#v, error=%v", projectTeam, err)
	}
	local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if relationshipID(local["package"]) != "package-2" || relationshipID(local["labels"].([]any)[0]) != "label-2" {
		t.Fatalf("remote task assignments were not pulled: %#v", local)
	}
	local["labels"] = []any{map[string]any{"id": "label-local", "label": "Local label"}}
	local["package"] = map[string]any{"id": "package-local", "label": "Local package"}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
		t.Fatal(err)
	}
	remote["version"] = float64(3)
	remote["status"] = "inProgress"
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	local, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if relationshipID(local["package"]) != "package-local" || relationshipID(local["labels"].([]any)[0]) != "label-local" {
		t.Fatalf("independent local assignments were not preserved during pull: %#v", local)
	}
	if local["status"] != "inProgress" {
		t.Fatalf("remote-only status update was not merged: %#v", local)
	}
}
