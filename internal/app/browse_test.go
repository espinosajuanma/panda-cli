package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type browserResponse struct {
	value string
	err   error
}

type browserCall struct {
	prompt  string
	options []selectorOption
}

type scriptedBrowserUI struct {
	responses []browserResponse
	calls     []browserCall
}

func (browser *scriptedBrowserUI) Select(prompt string, options []selectorOption, _ string) (string, error) {
	copyOptions := append([]selectorOption(nil), options...)
	browser.calls = append(browser.calls, browserCall{prompt: prompt, options: copyOptions})
	if len(browser.responses) == 0 {
		return "", errors.New("unexpected selector request")
	}
	response := browser.responses[0]
	browser.responses = browser.responses[1:]
	return response.value, response.err
}

func TestBrowseDisplaysReadOnlyTaskActionsAndReturnsToSelector(t *testing.T) {
	remote := map[string]any{
		"id": "browse-task", "number": float64(42), "title": "Browsable task", "type": "feature",
		"priority": "high", "status": "inProgress", "description": "<p>Browse body</p>",
		"project": map[string]any{"id": "project-1", "label": "Alpha"},
		"labels":  []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}, "files": []any{},
	}
	ws, dir := makeSyncedTask(t, remote)
	noteName := "20261102T120000.000Z-local.md"
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "notes", noteName), []byte("Browser note"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("browse-task")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{noteName}
	if err := ws.WriteState("browse-task", state); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(dir, "metadata.json")
	descriptionPath := filepath.Join(dir, "description.md")
	statePath := ws.StatePathForKey("browse-task")
	beforeMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeDescription, err := os.ReadFile(descriptionPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	browser := &scriptedBrowserUI{responses: []browserResponse{
		{value: "42"},
		{value: "view"},
		{value: "description"},
		{value: "notes"},
		{value: "files"},
		{value: "diff"},
		{value: "link"},
		{value: "back"},
		{err: errSelectionCanceled},
	}}
	var output strings.Builder
	if err := browseWithUI(ws, nil, "", config.DefaultAPIURL, browser, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Browsable task", "Browse body", "Browser note", "No attached files.",
		"staged note:", "[#42]",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("browser output missing %q:\n%s", expected, output.String())
		}
	}
	if len(browser.calls) != 9 || browser.calls[0].prompt != "Tasks · type to filter" || browser.calls[1].prompt != "Task actions" {
		t.Fatalf("browser selector calls = %#v", browser.calls)
	}
	backAvailable := false
	for _, option := range browser.calls[1].options {
		if option.value == "back" {
			backAvailable = true
		}
		if option.value == "edit" || option.value == "push" || option.value == "discard" {
			t.Fatalf("read-only browser exposed a mutating action: %#v", option)
		}
	}
	if !backAvailable {
		t.Fatal("browser action menu is missing Back to tasks")
	}
	afterMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	afterDescription, err := os.ReadFile(descriptionPath)
	if err != nil {
		t.Fatal(err)
	}
	afterState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeMetadata, afterMetadata) || !bytes.Equal(beforeDescription, afterDescription) || !bytes.Equal(beforeState, afterState) {
		t.Fatal("browser modified local task data or sync state")
	}
	if _, err := os.Stat(filepath.Join(ws.Root, ".panda", "workspace.lock")); !os.IsNotExist(err) {
		t.Fatalf("read-only browser created a workspace lock: %v", err)
	}
}

func TestBrowseAppliesListFiltersAndHandlesNoMatches(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectMembers(ws, []map[string]any{{"user": map[string]any{"id": "user-ada", "label": "Ada", "email": "ada@example.com"}, "role": "developer"}}); err != nil {
		t.Fatal(err)
	}
	assigned := listTask("browse-assigned", 51, "inProgress", "a", "s", "Assigned browse task", "feature")
	assigned["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada"}}
	unassigned := listTask("browse-unassigned", 52, "inProgress", "b", "m", "Unassigned browse task", "feature")
	addSyncedTask(t, ws, assigned)
	addSyncedTask(t, ws, unassigned)
	browser := &scriptedBrowserUI{responses: []browserResponse{{value: "51"}, {err: errSelectionCanceled}}}
	if err := browseWithUI(ws, []string{"--assignee", "me"}, "ada@example.com", config.DefaultAPIURL, browser, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(browser.calls) != 2 || len(browser.calls[0].options) != 1 || browser.calls[0].options[0].value != "51" {
		t.Fatalf("filtered browser task choices = %#v", browser.calls)
	}
	empty := workspace.New(t.TempDir())
	if err := empty.Ensure(); err != nil {
		t.Fatal(err)
	}
	noTasksBrowser := &scriptedBrowserUI{}
	var output strings.Builder
	if err := browseWithUI(empty, nil, "", config.DefaultAPIURL, noTasksBrowser, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "No tasks match the filters.\n" || len(noTasksBrowser.calls) != 0 {
		t.Fatalf("empty browser output/calls = %q / %#v", output.String(), noTasksBrowser.calls)
	}
}

func TestBrowseRejectsNonInteractiveAndJSONUse(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := browseTasks(ws, nil, "", config.DefaultAPIURL, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("noninteractive browse error = %v", err)
	}
	if err := browseWithUI(ws, []string{"--json"}, "", config.DefaultAPIURL, &scriptedBrowserUI{}, io.Discard); err == nil || !strings.Contains(err.Error(), "does not accept --json") {
		t.Fatalf("browse JSON error = %v", err)
	}
}

func TestReadOnlyProjectWorkspaceDoesNotInitializeMissingWorkspace(t *testing.T) {
	setTestConfigHome(t)
	cfg := &config.Config{Project: &config.Project{ID: "project-1", Name: "Not Initialized"}}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readOnlyProjectWorkspace(cfg); err == nil {
		t.Fatal("missing workspace unexpectedly opened")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("read-only workspace lookup created the path: %v", err)
	}
}
