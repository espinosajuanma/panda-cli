package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/workspace"
)

func TestDiffGroupsTaskChangesAndShowsDescriptionLines(t *testing.T) {
	first := map[string]any{"id": "task-diff-1", "number": float64(13359), "title": "Improve task diffs", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Original body</p>", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}}
	second := map[string]any{"id": "task-diff-2", "number": float64(13360), "title": "Unchanged task", "type": "feature", "priority": "medium", "status": "new", "description": "<p>Same body</p>", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}}
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
	firstHeader, secondHeader := strings.Index(allTasks, "Task 13359: Improve task diffs"), strings.Index(allTasks, "Task 13360: Changed second title")
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
	for _, expected := range []string{"Task 13359: Improve task diffs", "priority: medium -> high", "description: updated"} {
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
	for _, expected := range []string{"description.md", "-Original body", "+Updated body", "+A new line"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("verbose diff output missing %q:\n%s", expected, output.String())
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
	remote := map[string]any{"id": "task-note-diff", "number": float64(13765), "title": "Follow up on partnership request", "type": "feature", "priority": "medium", "status": "new", "description": "", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{}}
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
	for _, expected := range []string{"Task 13765: Follow up on partnership request", "staged note: notes/" + filename, "+Reached out to the partner.", "+They will reply next week."} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("verbose diff output missing %q:\n%s", expected, output.String())
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
	remote := map[string]any{"id": "task-note-edit", "number": float64(13766), "title": "Edit a synced note", "type": "feature", "priority": "medium", "status": "new", "description": "", "project": map[string]any{"id": "project-1"}, "labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{map[string]any{"note": "<p>Original note body</p>", "timestamp": float64(timestamp), "addedBy": map[string]any{"id": "user-1", "label": "Juan"}}}}
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
	metadata["priority"], metadata["complexity"] = "high", "xl"
	if err := workspace.WriteMap(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := diff(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"new task", "type: feature", "priority: high", "complexity: xl"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("new task diff missing %q:\n%s", expected, output.String())
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
	for dir, updates := range map[string]map[string]any{editedDir: {"title": "Updated title"}, discardedDir: {"status": "discarded"}} {
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
	if shouldPageDiff([]string{"--summary"}, true, []byte("summary\n"), 80, 24) || shouldPageDiff([]string{"-s"}, true, []byte("summary\n"), 80, 24) {
		t.Fatal("summary diff should not use the pager")
	}
	if shouldPageDiff([]string{"--verbose"}, true, []byte("small diff\n"), 80, 24) || !shouldPageDiff([]string{"--verbose"}, true, []byte(strings.Repeat("line\n", 25)), 80, 24) {
		t.Fatal("pager threshold did not distinguish short and long detail diffs")
	}
}

func TestDiffShortFlagsMatchLongFlags(t *testing.T) {
	verbose, err := parseDiffOptions([]string{"-v"})
	if err != nil || !verbose.verbose {
		t.Fatalf("-v options = %#v, error = %v; want verbose mode", verbose, err)
	}
	summary, err := parseDiffOptions([]string{"-s"})
	if err != nil || !summary.summary {
		t.Fatalf("-s options = %#v, error = %v; want summary mode", summary, err)
	}
	combined, err := parseDiffOptions([]string{"12345", "-v", "-s"})
	if err != nil || combined.task != "12345" || !combined.verbose || !combined.summary {
		t.Fatalf("combined short flags = %#v, error = %v", combined, err)
	}
}

func TestOutputFitsScreenAccountsForWrappingAndANSI(t *testing.T) {
	if !outputFitsScreen([]byte("\x1b[1;36mheading\x1b[0m\nbody\n"), 10, 2) || outputFitsScreen([]byte("12345678901\n"), 10, 1) {
		t.Fatal("screen fitting did not account for ANSI controls or wrapped lines")
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

func TestConfigureLessPagerKeepsColorsAndOutputVisible(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"adds raw color and no-clear options", []string{"less"}, []string{"less", "-R", "-X"}},
		{"preserves existing combined option", []string{"less", "-FRX"}, []string{"less", "-FRX"}},
		{"preserves combined raw option", []string{"less", "-FrX"}, []string{"less", "-FrX"}},
		{"adds no-clear to existing color option", []string{"less", "-F", "-R"}, []string{"less", "-F", "-R", "-X"}},
		{"recognizes long no-init option", []string{"less", "--raw-control-chars", "--no-init"}, []string{"less", "--raw-control-chars", "--no-init"}},
		{"leaves other pagers unchanged", []string{"more"}, []string{"more"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := configureLessPager(test.args); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("configureLessPager(%v) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}
