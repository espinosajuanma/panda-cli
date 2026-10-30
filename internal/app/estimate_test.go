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
	bug := listTask("estimate-new-bug", 205, "new", "b", nil, "Bug to size", "bug")
	other := listTask("estimate-new-other", 206, "new", "c", nil, "Other to size", "other")
	otherStatus := listTask("estimate-todo-missing", 203, "toDo", "a", nil, "To do task", "feature")
	release := listTask("estimate-release", 204, "new", "d", nil, "Release task", "release")
	for _, fixture := range []map[string]any{missing, existing, bug, other, otherStatus, release} {
		addSyncedTask(t, ws, fixture)
	}

	var output strings.Builder
	if err := estimateTasks(ws, nil, strings.NewReader("m\ns\nl\n"), &output); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath("201"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := stringField(metadata, "complexity"); got != "m" {
		t.Fatalf("assigned complexity = %q, want m", got)
	}
	for taskID, want := range map[string]string{"205": "s", "206": "l"} {
		metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath(taskID), "metadata.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := stringField(metadata, "complexity"); got != want {
			t.Errorf("default type selection assigned task %s complexity %q, want %q", taskID, got, want)
		}
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
		"Estimate complete: 3 updated, 0 skipped, 0 tasks still without complexity in new (types: feature, bug, other).",
		"Run `panda push`",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("estimate output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Already sized") || strings.Contains(text, "To do task") || strings.Contains(text, "Release task") {
		t.Fatalf("default estimate included an ineligible task:\n%s", text)
	}
}

func TestEstimateTypeFilterAndExplicitReleaseSelection(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("estimate-type-feature", 601, "new", "a", nil, "Feature task", "feature"),
		listTask("estimate-type-bug", 602, "new", "b", nil, "Bug task", "bug"),
		listTask("estimate-type-other", 603, "new", "c", nil, "Other task", "other"),
		listTask("estimate-type-release", 604, "new", "d", nil, "Release task", "release"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}

	var output strings.Builder
	if err := estimateTasks(ws, []string{"--type", "bug,other"}, strings.NewReader("m\nxl\n"), &output); err != nil {
		t.Fatal(err)
	}
	for taskID, want := range map[string]string{"601": "", "602": "m", "603": "xl", "604": ""} {
		metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath(taskID), "metadata.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := stringField(metadata, "complexity"); got != want {
			t.Errorf("task %s complexity = %q, want %q", taskID, got, want)
		}
	}
	if strings.Contains(output.String(), "Feature task") || strings.Contains(output.String(), "Release task") {
		t.Fatalf("type filter included unselected types:\n%s", output.String())
	}

	output.Reset()
	if err := estimateTasks(ws, []string{"--type=release"}, strings.NewReader("s\n"), &output); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(ws.TaskPath("604"), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := stringField(metadata, "complexity"); got != "s" {
		t.Fatalf("explicit release type complexity = %q, want s", got)
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
	if !strings.Contains(output.String(), "Estimate complete: 3 updated, 0 skipped, 0 tasks still without complexity in new, toDo (types: feature, bug, other).") {
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
	if !strings.Contains(text, "Stopped early. Estimate complete: 0 updated, 1 skipped, 3 tasks still without complexity in new (types: feature, bug, other).") {
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
	if !strings.Contains(output.String(), "No matching tasks in status new need a complexity (types: feature, bug, other).") {
		t.Fatalf("no-work message is missing: %q", output.String())
	}
	if err := estimateTasks(ws, []string{"nwe"}, strings.NewReader(""), &output); err == nil || !strings.Contains(err.Error(), `unknown status "nwe"`) {
		t.Fatalf("invalid status error = %v", err)
	}
	if _, err := parseEstimateOptions([]string{"--unknown"}); err == nil || !strings.Contains(err.Error(), "usage: panda estimate") {
		t.Fatalf("unknown option error = %v", err)
	}
	if _, err := parseEstimateOptions([]string{"--type", "feat"}); err == nil || !strings.Contains(err.Error(), `unknown task type "feat"`) {
		t.Fatalf("invalid task type error = %v", err)
	}
}
