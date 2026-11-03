package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
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

func TestResponsiveListDropsOptionalColumnsAndWrapsTitles(t *testing.T) {
	task := listTask("responsive", 61, "new", "a", "m", "Implement a responsive terminal layout for wide task tables", "feature")
	task["package"] = map[string]any{"id": "package-1", "label": "Long package name"}
	task["labels"] = []any{map[string]any{"id": "label-1", "label": "long-label-name"}}
	task["assignees"] = []any{map[string]any{"id": "user-1", "label": "Ada Lovelace"}}
	task["reviewers"] = []any{map[string]any{"id": "user-2", "label": "Grace Hopper"}}

	var output strings.Builder
	if _, err := printTaskGroup(&output, &output, "new", []map[string]any{task}, nil, nil, true, true, 60); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, hidden := range []string{"PACKAGE", "LABELS", "ASSIGNEES", "REVIEWERS", "HOURS"} {
		if strings.Contains(text, hidden) {
			t.Errorf("narrow table retained lower-priority column %q:\n%s", hidden, text)
		}
	}
	for _, shown := range []string{"NUMBER", "PRIORITY", "TYPE", "COMPLEXITY", "SP", "TITLE"} {
		if !strings.Contains(text, shown) {
			t.Errorf("narrow table hid expected column %q:\n%s", shown, text)
		}
	}
	if !strings.Contains(text, "Implement") || !strings.Contains(text, "tables") {
		t.Fatalf("wrapped title content was lost:\n%s", text)
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if width := utf8.RuneCountInString(line); width > 60 {
			t.Errorf("responsive output line has %d columns, want at most 60: %q", width, line)
		}
	}
}

func TestListWideOverridesTerminalWidthAndCannotCombineWithJSON(t *testing.T) {
	if got := listTableWidth(false, true, 48); got != 48 {
		t.Fatalf("terminal list width = %d, want 48", got)
	}
	if got := listTableWidth(true, true, 48); got != 0 {
		t.Fatalf("wide list width = %d, want 0 (unconstrained)", got)
	}
	if got := listTableWidth(false, false, 48); got != 0 {
		t.Fatalf("piped list width = %d, want 0 (full table)", got)
	}
	options, err := parseListOptions([]string{"--wide"})
	if err != nil || !options.wide {
		t.Fatalf("--wide parsing: options=%#v error=%v", options, err)
	}
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := listTasks(ws, []string{"--wide", "--json"}, io.Discard); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("--wide --json error = %v", err)
	}
}

func TestListJSONIncludesSortedGroupsRelationshipsEffortAndTotals(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, labelCatalog, []map[string]any{
		{"id": "label-1", "name": "security"},
		{"id": "label-2", "name": "urgent"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalog(ws, packageCatalog, []map[string]any{{"id": "package-1", "title": "Billing"}}); err != nil {
		t.Fatal(err)
	}

	second := listTask("json-second", 42, "new", "b", "m", "Second task", "feature")
	second["package"] = map[string]any{"id": "package-1", "label": "Old package name"}
	second["labels"] = []any{
		map[string]any{"id": "label-2", "label": "Old urgent name"},
		map[string]any{"id": "label-1", "label": "Old security name"},
	}
	second["assignees"] = []any{map[string]any{"id": "user-2", "label": "Grace Hopper"}, map[string]any{"id": "user-1", "label": "Ada Lovelace"}}
	second["reviewers"] = []any{map[string]any{"id": "user-3", "label": "Katherine Johnson"}}
	first := listTask("json-first", 41, "new", "a", "unknown", "First task", "bug")
	otherStatus := listTask("json-hidden", 43, "discarded", "a", "xl", "Hidden task", "feature")
	for _, fixture := range []map[string]any{second, otherStatus, first} {
		addSyncedTask(t, ws, fixture)
	}

	var output bytes.Buffer
	if err := listTasks(ws, []string{"--json", "--status=new"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("JSON output contains ANSI styling: %q", output.String())
	}
	var document listJSONDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("invalid JSON output %q: %v", output.String(), err)
	}
	if document.SchemaVersion != 1 || len(document.Groups) != 1 {
		t.Fatalf("JSON schema/groups = %d / %#v", document.SchemaVersion, document.Groups)
	}
	group := document.Groups[0]
	if group.Status != "new" || len(group.Tasks) != 2 {
		t.Fatalf("JSON group = %#v", group)
	}
	if got := stringField(group.Tasks[0].Task, "id"); got != "json-first" {
		t.Fatalf("ranked first task ID = %q, want json-first", got)
	}
	secondTask := group.Tasks[1]
	if stringField(secondTask.Task, "id") != "json-second" || secondTask.StoryPoints == nil || *secondTask.StoryPoints != 3 || secondTask.Hours == nil || *secondTask.Hours != 12 {
		t.Fatalf("known-complexity JSON task = %#v", secondTask)
	}
	if secondTask.Package == nil || *secondTask.Package != "Billing" || !reflect.DeepEqual(secondTask.Labels, []string{"security", "urgent"}) {
		t.Fatalf("catalog relationship names = package %v, labels %v", secondTask.Package, secondTask.Labels)
	}
	if !reflect.DeepEqual(secondTask.Assignees, []string{"Ada Lovelace", "Grace Hopper"}) || !reflect.DeepEqual(secondTask.Reviewers, []string{"Katherine Johnson"}) {
		t.Fatalf("assignees/reviewers = %v / %v", secondTask.Assignees, secondTask.Reviewers)
	}
	if group.Tasks[0].StoryPoints != nil || group.Tasks[0].Hours != nil {
		t.Fatalf("unknown complexity should produce null effort values: %#v", group.Tasks[0])
	}
	if group.Totals != (listJSONTotals{Tasks: 2, StoryPoints: 3, Hours: 12}) || document.Totals != group.Totals {
		t.Fatalf("group/project totals = %#v / %#v", group.Totals, document.Totals)
	}
}
