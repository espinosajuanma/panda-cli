package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"panda-dashboard/internal/workspace"
)

func TestListGroupsByWorkflowStatusSortsByRankAndFilters(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("new-b", 12, "new", "b", "m", "New B", "feature"),
		listTask("new-a", 11, "new", "a", nil, "New A", "feature"),
		listTask("todo", 13, "toDo", "c", "", "To Do", "bug"),
		listTask("progress", 14, "inProgress", "d", "s", "In Progress", "feature"),
		listTask("review", 15, "inReview", "e", "m", "In Review", "feature"),
		listTask("complete", 16, "completed", "f", "l", "Completed", "feature"),
		listTask("staging", 17, "staging", "g", "xl", "Staging", "feature"),
		listTask("release", 18, "released", "h", "xxl", "Release", "release"),
		listTask("discard", 19, "discarded", "i", "s", "Discarded", "bug"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}
	if err := os.WriteFile(filepath.Join(ws.TaskPath("12"), "description.md"), []byte("Has a description"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.TaskPath("13"), "description.md"), []byte(" \n\t "), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	statusPositions := []int{
		strings.Index(text, "NEW\n"), strings.Index(text, "TO DO\n"), strings.Index(text, "IN PROGRESS\n"),
		strings.Index(text, "IN REVIEW\n"), strings.Index(text, "COMPLETED\n"),
	}
	for index, position := range statusPositions {
		if position < 0 || (index > 0 && position <= statusPositions[index-1]) {
			t.Fatalf("workflow statuses are not grouped in the requested order: %q", text)
		}
	}
	if got := strings.Count(text, "----------------------------------------"); got != len(statusPositions)-1 {
		t.Fatalf("got %d status dividers, want %d", got, len(statusPositions)-1)
	}
	for _, hiddenStatus := range []string{"STAGING\n", "RELEASED\n", "DISCARDED\n"} {
		if strings.Contains(text, hiddenStatus) {
			t.Fatalf("default list included excluded status %q: %q", hiddenStatus, text)
		}
	}
	if strings.Index(text, "New A") > strings.Index(text, "New B") {
		t.Fatalf("new tasks were not sorted by rank: %q", text)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--no-description"}, &output); err != nil {
		t.Fatal(err)
	}
	withoutDescriptions := output.String()
	if !strings.Contains(withoutDescriptions, "New A") || !strings.Contains(withoutDescriptions, "To Do") || strings.Contains(withoutDescriptions, "New B") {
		t.Fatalf("no-description filter did not keep blank descriptions and exclude populated ones: %q", withoutDescriptions)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--status", "new,toDo", "--no-complexity"}, &output); err != nil {
		t.Fatal(err)
	}
	filtered := output.String()
	if !strings.Contains(filtered, "New A") || !strings.Contains(filtered, "To Do") || strings.Contains(filtered, "New B") || strings.Contains(filtered, "Discarded") {
		t.Fatalf("status/no-complexity filters were not applied: %q", filtered)
	}

	output.Reset()
	if err := listTasks(ws, []string{"--status=discarded"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Discarded") {
		t.Fatalf("explicit discarded filter did not show discarded tasks: %q", output.String())
	}
}

func TestListQuickFiltersCoverAllStatusesAndCombine(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("quick-new", 31, "new", "a", "xs", "New task", "feature"),
		listTask("quick-todo", 32, "toDo", "a", "s", "To do task", "feature"),
		listTask("quick-progress", 33, "inProgress", "a", "m", "In progress task", "feature"),
		listTask("quick-review", 34, "inReview", "a", "l", "In review task", "feature"),
		listTask("quick-completed", 35, "completed", "a", "xl", "Completed task", "feature"),
		listTask("quick-staging", 36, "staging", "a", "xxl", "Staging task", "feature"),
		listTask("quick-released", 37, "released", "a", "xs", "Released task", "feature"),
		listTask("quick-discarded", 38, "discarded", "a", "s", "Discarded task", "feature"),
		listTask("quick-custom", 39, "blocked", "a", "m", "Custom status task", "feature"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}
	tests := []struct {
		name      string
		args      []string
		wantTasks []string
		omitTasks []string
	}{
		{name: "default active", wantTasks: []string{"New task", "To do task", "In progress task", "In review task", "Completed task"}, omitTasks: []string{"Staging task", "Released task", "Discarded task", "Custom status task"}},
		{name: "current", args: []string{"--current"}, wantTasks: []string{"To do task", "In progress task", "In review task", "Completed task"}, omitTasks: []string{"New task", "Staging task", "Released task", "Discarded task", "Custom status task"}},
		{name: "staging", args: []string{"--staging"}, wantTasks: []string{"Staging task"}, omitTasks: []string{"New task", "Released task", "Discarded task", "Custom status task"}},
		{name: "released", args: []string{"--released"}, wantTasks: []string{"Released task"}, omitTasks: []string{"New task", "Staging task", "Discarded task", "Custom status task"}},
		{name: "discarded", args: []string{"--discarded"}, wantTasks: []string{"Discarded task"}, omitTasks: []string{"New task", "Staging task", "Released task", "Custom status task"}},
		{name: "all includes custom statuses", args: []string{"--all"}, wantTasks: []string{"New task", "To do task", "In progress task", "In review task", "Completed task", "Staging task", "Released task", "Discarded task", "Custom status task"}},
		{name: "quick filters combine", args: []string{"--current", "--staging"}, wantTasks: []string{"To do task", "In progress task", "In review task", "Completed task", "Staging task"}, omitTasks: []string{"New task", "Released task", "Discarded task", "Custom status task"}},
		{name: "status and quick filter combine", args: []string{"--status", "new", "--released"}, wantTasks: []string{"New task", "Released task"}, omitTasks: []string{"Staging task", "Discarded task", "Custom status task"}},
		{name: "other filters remain composable", args: []string{"--current", "--no-complexity", "--no-description"}, omitTasks: []string{"New task", "Staging task", "Released task", "Discarded task", "Custom status task"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := listTasks(ws, test.args, &output); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			for _, want := range test.wantTasks {
				if !strings.Contains(text, want) {
					t.Errorf("list output missing %q:\n%s", want, text)
				}
			}
			for _, omit := range test.omitTasks {
				if strings.Contains(text, omit) {
					t.Errorf("list output unexpectedly contains %q:\n%s", omit, text)
				}
			}
		})
	}
}

func TestListShowsStoryPointsHoursAndVisibleTotals(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixtures := []map[string]any{
		listTask("new-xs", 21, "new", "a", "xs", "Extra small", "feature"),
		listTask("new-s", 22, "new", "b", "s", "Small", "feature"),
		listTask("new-unknown", 23, "new", "c", "unknown", "Unknown", "feature"),
		listTask("progress-m", 24, "inProgress", "a", "m", "Medium", "feature"),
		listTask("progress-l", 25, "inProgress", "b", "l", "Large", "feature"),
		listTask("progress-xl", 26, "inProgress", "c", "xl", "Extra large", "feature"),
		listTask("progress-xxl", 27, "inProgress", "d", "xxl", "Double extra large", "feature"),
		listTask("discarded-xs", 28, "discarded", "a", "xs", "Hidden by default", "feature"),
	}
	for _, fixture := range fixtures {
		addSyncedTask(t, ws, fixture)
	}
	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"COMPLEXITY", "SP", "HOURS", "PACKAGE", "LABELS", "xs", "1", "4", "xxl", "13", "52", "unknown", "Subtotal: 3 SP, 12 hours", "Subtotal: 29 SP, 116 hours", "TOTAL (visible tasks): 32 SP, 128 hours"} {
		if !strings.Contains(text, expected) {
			t.Errorf("list output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Hidden by default") {
		t.Fatalf("default filtering or unknown-complexity row was incorrect:\n%s", text)
	}
	unknownRowFound := false
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 7 && fields[0] == "23" {
			unknownRowFound = reflect.DeepEqual(fields[:9], []string{"23", "medium", "feature", "unknown", "-", "-", "-", "-", "Unknown"})
			break
		}
	}
	if !unknownRowFound {
		t.Fatalf("unrecognized complexity should show dashes in both calculated columns:\n%s", text)
	}
	output.Reset()
	if err := listTasks(ws, []string{"--status", "new"}, &output); err != nil {
		t.Fatal(err)
	}
	filtered := output.String()
	if !strings.Contains(filtered, "Subtotal: 3 SP, 12 hours") || !strings.Contains(filtered, "TOTAL (visible tasks): 3 SP, 12 hours") || strings.Contains(filtered, "Medium") {
		t.Fatalf("filtered list totals should include only visible tasks:\n%s", filtered)
	}
}

func TestListShowsPackageAndLabelRelationshipsAndFilters(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	withAssignment := listTask("with-tags", 51, "new", "a", "s", "Tagged task", "feature")
	withAssignment["package"] = map[string]any{"id": "p1", "label": "Billing"}
	withAssignment["labels"] = []any{map[string]any{"id": "l2", "label": "urgent"}, map[string]any{"id": "l1", "label": "security"}}
	withoutPackage := listTask("no-package", 52, "new", "b", "m", "Unpackaged task", "feature")
	withoutPackage["labels"] = []any{map[string]any{"id": "l3", "label": "triage"}}
	withoutLabels := listTask("no-labels", 53, "new", "c", "l", "Unlabeled task", "feature")
	withoutLabels["package"] = map[string]any{"id": "p2", "label": "Platform"}
	for _, fixture := range []map[string]any{withAssignment, withoutPackage, withoutLabels} {
		addSyncedTask(t, ws, fixture)
	}
	var output strings.Builder
	if err := listTasks(ws, nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"PACKAGE", "LABELS", "Billing", "security, urgent", "Platform", "triage"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("list output missing %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	if err := listTasks(ws, []string{"--no-package"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Unpackaged task") || strings.Contains(output.String(), "Tagged task") || strings.Contains(output.String(), "Unlabeled task") {
		t.Fatalf("--no-package did not select only tasks without a package:\n%s", output.String())
	}
	output.Reset()
	if err := listTasks(ws, []string{"--no-labels"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Unlabeled task") || strings.Contains(output.String(), "Tagged task") || strings.Contains(output.String(), "Unpackaged task") {
		t.Fatalf("--no-labels did not select only tasks without labels:\n%s", output.String())
	}
}
