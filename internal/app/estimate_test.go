package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestEstimateDefaultsToNewTasksWithoutComplexity(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	missing := listTask("estimate-new-missing", 201, "new", "a", nil, "Needs sizing", "feature")
	existing := listTask("estimate-new-existing", 202, "new", "b", "s", "Already sized", "bug")
	otherStatus := listTask("estimate-todo-missing", 203, "toDo", "a", nil, "To do task", "feature")
	for _, fixture := range []map[string]any{missing, existing, otherStatus} {
		addSyncedTask(t, ws, fixture)
	}

	var output strings.Builder
	if err := estimateTasks(ws, nil, strings.NewReader("m\n"), &output); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath("201"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := stringField(metadata, "complexity"); got != "m" {
		t.Fatalf("assigned complexity = %q, want m", got)
	}
	metadata, err = workspace.ReadMap(filepath.Join(ws.TaskPath("202"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := stringField(metadata, "complexity"); got != "s" {
		t.Fatalf("existing complexity changed to %q", got)
	}
	metadata, err = workspace.ReadMap(filepath.Join(ws.TaskPath("203"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !isEmptyField(metadata["complexity"]) {
		t.Fatalf("task in another status was updated: %#v", metadata["complexity"])
	}
	text := output.String()
	for _, expected := range []string{
		"#201 — Needs sizing", "xs — 1 SP / 4 hours", "s — 2 SP / 8 hours",
		"m — 3 SP / 12 hours", "l — 5 SP / 20 hours", "xl — 8 SP / 32 hours", "xxl — 13 SP / 52 hours",
		"Estimate complete: 1 updated, 0 skipped, 0 tasks still without complexity in new.",
		"Run `panda push`",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("estimate output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Already sized") || strings.Contains(text, "To do task") {
		t.Fatalf("default estimate included an ineligible task:\n%s", text)
	}
}

func TestEstimateAcceptsMultipleStatusesAndAllReestimates(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("estimate-all-new", 301, "new", "a", nil, "New task", "feature"),
		listTask("estimate-all-todo-empty", 302, "toDo", "a", nil, "Unestimated to do", "feature"),
		listTask("estimate-all-todo-existing", 303, "toDo", "b", "s", "Existing to do", "bug"),
		listTask("estimate-all-review", 304, "inReview", "a", nil, "Review task", "feature"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}

	var output strings.Builder
	if err := estimateTasks(ws, []string{"toDo", "new", "toDo", "--all"}, strings.NewReader("xs\nl\nxl\n"), &output); err != nil {
		t.Fatal(err)
	}
	for taskID, want := range map[string]string{"301": "xs", "302": "l", "303": "xl"} {
		metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath(taskID), "metadata.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := stringField(metadata, "complexity"); got != want {
			t.Errorf("task %s complexity = %q, want %q", taskID, got, want)
		}
	}
	if strings.Contains(output.String(), "Review task") {
		t.Fatalf("unselected status was included:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "Estimate complete: 3 updated, 0 skipped, 0 tasks still without complexity in new, toDo.") {
		t.Fatalf("missing multi-status summary:\n%s", output.String())
	}
}

func TestEstimateCanPreviewDescriptionSkipAndQuit(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []map[string]any{
		listTask("estimate-preview-1", 401, "new", "a", nil, "First task", "feature"),
		listTask("estimate-preview-2", 402, "new", "b", nil, "Second task", "feature"),
		listTask("estimate-preview-3", 403, "new", "c", nil, "Third task", "feature"),
	} {
		addSyncedTask(t, ws, fixture)
	}
	if err := os.WriteFile(filepath.Join(ws.TaskPath("401"), "description.md"), []byte("Description body to review."), 0o600); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := estimateTasks(ws, nil, strings.NewReader("d\n\nq\n"), &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Description body to review.") {
		t.Fatalf("description preview missing from output:\n%s", text)
	}
	if !strings.Contains(text, "Stopped early. Estimate complete: 0 updated, 1 skipped, 3 tasks still without complexity in new.") {
		t.Fatalf("skip/quit summary is incorrect:\n%s", text)
	}
	if strings.Contains(text, "#403 — Third task") {
		t.Fatalf("quit did not stop before the final task:\n%s", text)
	}
}

func TestEstimateReportsNoWorkAndRejectsInvalidStatuses(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	addSyncedTask(t, ws, listTask("estimate-complete", 501, "new", "a", "m", "Already complete", "feature"))
	var output strings.Builder
	if err := estimateTasks(ws, nil, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No tasks in status new need a complexity.") {
		t.Fatalf("no-work message is missing: %q", output.String())
	}
	if err := estimateTasks(ws, []string{"nwe"}, strings.NewReader(""), &output); err == nil || !strings.Contains(err.Error(), `unknown status "nwe"`) {
		t.Fatalf("invalid status error = %v", err)
	}
	if _, err := parseEstimateOptions([]string{"--unknown"}); err == nil || !strings.Contains(err.Error(), "usage: panda estimate") {
		t.Fatalf("unknown option error = %v", err)
	}
}
