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

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func releaseNotesRemote(id string, number float64, customer, internal string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "title": "Release notes task", "type": "release",
		"priority": "medium", "status": "inProgress", "description": "",
		"project":   map[string]any{"id": "project-1", "label": "Alpha"},
		"labels":    []any{},
		"assignees": []any{},
		"reviewers": []any{},
		"notes":     []any{},
		"releaseNotes": map[string]any{
			"customerNotes": customer,
			"internalNotes": internal,
		},
	}
}

func readReleaseNoteFile(t *testing.T, dir, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReleaseNoteHelpersDetectChangesConflictsAndAdvanceBaseline(t *testing.T) {
	baseline := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	dir := t.TempDir()
	if err := writeReleaseNotesToLocal(dir, baseline); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Local customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	changes, err := releaseNoteChanges(dir, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes["customerNotes"] != "Local customer" {
		t.Fatalf("release note changes = %#v", changes)
	}
	conflicts, err := releaseNoteConflicts(dir, baseline, baseline)
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("unchanged remote produced conflicts %v, error %v", conflicts, err)
	}
	remote := cloneMap(baseline)
	remote["releaseNotes"].(map[string]any)["customerNotes"] = "<p>Remote customer</p>"
	conflicts, err = releaseNoteConflicts(dir, baseline, remote)
	if err != nil || len(conflicts) != 1 || conflicts[0] != releaseCustomerNotesFile {
		t.Fatalf("release note conflicts = %v, error %v", conflicts, err)
	}
	advanced := advanceReleaseNotesBaseline(baseline, remote, conflicts)
	notes := releaseNotesObject(advanced)
	if stringField(notes, "customerNotes") != "<p>Base customer</p>" || stringField(notes, "internalNotes") != "<p>Base internal</p>" {
		t.Fatalf("advanced release notes baseline = %#v", notes)
	}
}

func TestPullWritesReleaseNotesAndPreservesNonConflictingLocalEdits(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	remote["version"] = float64(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/projects/project-1":
			_, _ = fmt.Fprint(w, `{"members":[]}`)
		case r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages"):
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks":
			_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
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
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseCustomerNotesFile)); got != "Base customer" {
		t.Fatalf("customer notes = %q", got)
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseInternalNotesFile)); got != "Base internal" {
		t.Fatalf("internal notes = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Local customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	remote["version"] = float64(2)
	remote["releaseNotes"].(map[string]any)["internalNotes"] = "<p>Remote internal</p>"
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseCustomerNotesFile)); got != "Local customer" {
		t.Fatalf("local customer notes were overwritten: %q", got)
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseInternalNotesFile)); got != "Remote internal" {
		t.Fatalf("remote internal notes were not written: %q", got)
	}
}

func TestPullReportsReleaseNoteConflicts(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	remote["version"] = float64(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/projects/project-1":
			_, _ = fmt.Fprint(w, `{"members":[]}`)
		case r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages"):
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks":
			_, _ = fmt.Fprintf(w, `{"total":1,"items":[%s]}`, marshalForTest(remote))
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
	if err := pull(context.Background(), client, ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	dir := ws.TaskPath("14")
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Local customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	remote["version"] = float64(2)
	remote["releaseNotes"].(map[string]any)["customerNotes"] = "<p>Remote customer</p>"
	var output strings.Builder
	err := pull(context.Background(), client, ws, project, &output)
	if err == nil || !strings.Contains(output.String(), releaseCustomerNotesFile) {
		t.Fatalf("pull conflict result = %v, output %q", err, output.String())
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseCustomerNotesFile)); got != "Local customer" {
		t.Fatalf("conflicted local customer notes changed: %q", got)
	}
	state, stateErr := ws.ReadState("task-1")
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if got := stringField(releaseNotesObject(state.Remote), "customerNotes"); got != "<p>Base customer</p>" {
		t.Fatalf("conflicted baseline customer notes = %q", got)
	}
}

func TestPushEditsReleaseNotesThroughAction(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	var actionPayload map[string]string
	editCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1/editReleaseNotes":
			editCalls++
			if err := json.NewDecoder(r.Body).Decode(&actionPayload); err != nil {
				t.Error(err)
			}
			remote["releaseNotes"].(map[string]any)["customerNotes"] = actionPayload["customerNotes"]
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, remote)
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Changed customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if editCalls != 1 {
		t.Fatalf("editReleaseNotes calls = %d", editCalls)
	}
	if !strings.Contains(actionPayload["customerNotes"], "Changed customer") || actionPayload["internalNotes"] != "" {
		t.Fatalf("release notes payload = %#v", actionPayload)
	}
	state, err := ws.ReadState("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stringField(releaseNotesObject(state.Remote), "customerNotes"), "Changed customer") {
		t.Fatalf("release notes baseline = %#v", releaseNotesObject(state.Remote))
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseCustomerNotesFile)); got != "Changed customer" {
		t.Fatalf("local customer notes = %q", got)
	}
}

func TestPushRejectsReleaseNoteConflicts(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Remote customer</p>", "<p>Base internal</p>")
	editCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/data/dev.tasks/task-1":
			_ = json.NewEncoder(w).Encode(remote)
		case r.Method == http.MethodPut && r.URL.Path == "/data/dev.tasks/task-1/editReleaseNotes":
			editCalls++
			_ = json.NewEncoder(w).Encode(remote)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ws, dir := makeSyncedTask(t, releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>"))
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Local customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, &config.Project{ID: "project-1", Name: "Alpha"}, io.Discard); err == nil {
		t.Fatal("expected a release note conflict")
	}
	if editCalls != 0 {
		t.Fatalf("conflicting push made %d editReleaseNotes call(s)", editCalls)
	}
}

func TestPushCreatedTaskIncludesReleaseNotes(t *testing.T) {
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/data/dev.labels" || r.URL.Path == "/data/dev.packages"):
			_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/data/dev.tasks":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Error(err)
			}
			created["id"], created["number"], created["version"], created["notes"] = "remote-created", float64(77), float64(1), []any{}
			_ = json.NewEncoder(w).Encode(created)
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{ID: "project-1", Name: "Alpha"}
	if err := createTask(ws, project, []string{"--title", "Draft release"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	dirs, err := ws.TaskDirectories()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("draft directories = %v, error %v", dirs, err)
	}
	if err := os.WriteFile(filepath.Join(dirs[0], releaseCustomerNotesFile), []byte("Customer preview"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := push(context.Background(), solutions.NewClient(server.URL, "token"), ws, project, io.Discard); err != nil {
		t.Fatal(err)
	}
	notes, _ := created["releaseNotes"].(map[string]any)
	if !strings.Contains(stringField(notes, "customerNotes"), "Customer preview") {
		t.Fatalf("created releaseNotes = %#v", created["releaseNotes"])
	}
}

func TestDiffAndViewShowReleaseNotes(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	ws, dir := makeSyncedTask(t, remote)
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Updated customer\nSecond line"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := diff(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "customer notes:") || !strings.Contains(output.String(), releaseCustomerNotesFile+" updated") {
		t.Fatalf("default diff missing release note summary:\n%s", output.String())
	}
	output.Reset()
	if err := diff(ws, []string{"--verbose"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"customer notes: " + releaseCustomerNotesFile, "-Base customer", "+Updated customer", "+Second line"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("verbose diff missing %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	if err := viewTask(ws, []string{"14"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"## Customer notes", "Updated customer", "## Internal notes", "Base internal"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("view missing %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	if err := viewTask(ws, []string{"14", "--no-release-notes"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "## Customer notes") || strings.Contains(output.String(), "## Internal notes") {
		t.Fatalf("view did not suppress release notes:\n%s", output.String())
	}
	output.Reset()
	if err := viewTask(ws, []string{"14", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var document struct {
		ReleaseNotes struct {
			CustomerNotes string `json:"customerNotes"`
			InternalNotes string `json:"internalNotes"`
		} `json:"releaseNotes"`
	}
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.ReleaseNotes.CustomerNotes, "Updated customer") || !strings.Contains(document.ReleaseNotes.InternalNotes, "Base internal") {
		t.Fatalf("view JSON release notes = %#v", document.ReleaseNotes)
	}
}

func TestCleanRestoresReleaseNotes(t *testing.T) {
	remote := releaseNotesRemote("task-1", 14, "<p>Base customer</p>", "<p>Base internal</p>")
	ws, dir := makeSyncedTask(t, remote)
	if err := os.WriteFile(filepath.Join(dir, releaseCustomerNotesFile), []byte("Changed customer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, releaseInternalNotesFile), []byte("Changed internal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clean(ws, []string{"14"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseCustomerNotesFile)); got != "Base customer" {
		t.Fatalf("customer notes after clean = %q", got)
	}
	if got := strings.TrimSpace(readReleaseNoteFile(t, dir, releaseInternalNotesFile)); got != "Base internal" {
		t.Fatalf("internal notes after clean = %q", got)
	}
}

func TestReleaseNotesHTMLRoundTrip(t *testing.T) {
	html, err := content.MarkdownToHTML("Hello **world**")
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := content.HTMLToMarkdown(html)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "Hello **world**") {
		t.Fatalf("markdown round trip = %q", markdown)
	}
}
