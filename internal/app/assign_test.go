package app

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type scriptedAssignmentInteraction struct {
	actions      []rune
	selections   []map[string]bool
	initialState []map[string]bool
}

func (interaction *scriptedAssignmentInteraction) Action() (rune, error) {
	if len(interaction.actions) == 0 {
		return rune(selectorCancel), nil
	}
	action := interaction.actions[0]
	interaction.actions = interaction.actions[1:]
	return action, nil
}

func (interaction *scriptedAssignmentInteraction) SelectAssignees(_ []selectorOption, selected map[string]bool) (map[string]bool, error) {
	interaction.initialState = append(interaction.initialState, cloneBoolMap(selected))
	if len(interaction.selections) == 0 {
		return nil, nil
	}
	selection := interaction.selections[0]
	interaction.selections = interaction.selections[1:]
	return cloneBoolMap(selection), nil
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func TestParseAssignOptionsRequiresStatusAndValidatesFilters(t *testing.T) {
	options, err := parseAssignOptions([]string{"--status", "inProgress,toDo", "--priority=high,medium", "--type", "bug", "--type=feature"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(strings.Join(options.statuses, ","), "toDo,inProgress") {
		t.Fatalf("statuses = %v, want workflow order [toDo inProgress]", options.statuses)
	}
	if strings.Join(options.priorities, ",") != "high,medium" || strings.Join(options.types, ",") != "feature,bug" {
		t.Fatalf("priority/type filters = %v / %v", options.priorities, options.types)
	}
	for _, args := range [][]string{
		{"--priority", "high"},
		{"--status", "blocked"},
		{"--status", "toDo", "--priority", "urgent"},
		{"--status", "toDo", "--type", "task"},
	} {
		if _, err := parseAssignOptions(args); err == nil {
			t.Errorf("parseAssignOptions(%v) unexpectedly succeeded", args)
		}
	}
}

func TestFindAssignmentTasksFiltersAndSortsByStatusThenRank(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []map[string]any{
		listTask("assign-todo-b", 71, "toDo", "b", "m", "To do B", "bug"),
		listTask("assign-todo-a", 70, "toDo", "a", "l", "To do A", "bug"),
		listTask("assign-progress", 72, "inProgress", "a", "s", "Progress", "bug"),
		listTask("assign-wrong-priority", 73, "toDo", "c", "xs", "Low priority", "bug"),
		listTask("assign-wrong-type", 74, "toDo", "d", "xs", "Feature", "feature"),
		listTask("assign-completed", 75, "completed", "a", "xxl", "Completed", "bug"),
	} {
		addSyncedTask(t, ws, fixture)
	}
	options, err := parseAssignOptions([]string{"--status", "inProgress,toDo", "--priority", "high", "--type", "bug"})
	if err != nil {
		t.Fatal(err)
	}
	// Add the priority to the intended matches and keep a low-priority task filtered out.
	for _, number := range []string{"70", "71", "72"} {
		dir, metadata, err := ws.FindTask(number)
		if err != nil {
			t.Fatal(err)
		}
		metadata["priority"] = "high"
		if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
			t.Fatal(err)
		}
	}
	tasks, _, err := findAssignmentTasks(ws, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("matched %d tasks, want 3", len(tasks))
	}
	got := []string{stringField(tasks[0].metadata, "number"), stringField(tasks[1].metadata, "number"), stringField(tasks[2].metadata, "number")}
	if strings.Join(got, ",") != "70,71,72" {
		t.Fatalf("task order = %v, want rank order within workflow statuses", got)
	}
}

func TestFindAssignmentTasksSkipsReleaseByDefaultAndIncludesOnRequest(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []map[string]any{
		listTask("assign-feature", 76, "toDo", "a", "m", "Feature", "feature"),
		listTask("assign-release", 77, "toDo", "b", "l", "Release", "release"),
	} {
		addSyncedTask(t, ws, fixture)
	}
	defaultTypes, err := parseAssignOptions([]string{"--status", "toDo"})
	if err != nil {
		t.Fatal(err)
	}
	tasks, releaseSkipped, err := findAssignmentTasks(ws, defaultTypes)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || stringField(tasks[0].metadata, "type") != "feature" {
		t.Fatalf("default assignment candidates = %#v, want feature only", tasks)
	}
	if !releaseSkipped {
		t.Fatal("filtered release task was not reported as skipped")
	}

	releaseOnly, err := parseAssignOptions([]string{"--status", "toDo", "--type", "release"})
	if err != nil {
		t.Fatal(err)
	}
	tasks, releaseSkipped, err = findAssignmentTasks(ws, releaseOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || stringField(tasks[0].metadata, "type") != "release" {
		t.Fatalf("explicit release candidates = %#v, want release only", tasks)
	}
	if releaseSkipped {
		t.Fatal("explicit --type release was still reported as skipped")
	}

	featureAndRelease, err := parseAssignOptions([]string{"--status", "toDo", "--type", "feature,release"})
	if err != nil {
		t.Fatal(err)
	}
	tasks, releaseSkipped, err = findAssignmentTasks(ws, featureAndRelease)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("explicit feature/release candidates = %#v, want both tasks", tasks)
	}
	if releaseSkipped {
		t.Fatal("explicitly selected release type was still reported as skipped")
	}
}

func TestAssignFilteredTasksReplacesAssigneesAndSummarizesEffort(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	first := listTask("assign-first", 81, "toDo", "a", "m", "First task", "feature")
	second := listTask("assign-second", 82, "toDo", "b", "l", "Second task", "bug")
	second["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	addSyncedTask(t, ws, first)
	addSyncedTask(t, ws, second)
	options, err := parseAssignOptions([]string{"--status", "toDo"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, _, err := findAssignmentTasks(ws, options)
	if err != nil {
		t.Fatal(err)
	}
	members := []map[string]any{
		{"user": map[string]any{"id": "user-ada", "label": "Ada Lovelace"}, "role": "developer"},
		{"user": map[string]any{"id": "user-grace", "label": "Grace Hopper"}, "role": "qa"},
	}
	interaction := &scriptedAssignmentInteraction{
		actions:    []rune{rune(selectorEnter), rune(selectorEnter)},
		selections: []map[string]bool{{"user-ada": true}, {"user-grace": true}},
	}
	var output strings.Builder
	if err := assignFilteredTasks(options, candidates, members, interaction, &output); err != nil {
		t.Fatal(err)
	}
	_, firstMetadata, err := ws.FindTask("81")
	if err != nil {
		t.Fatal(err)
	}
	if assigneesDisplay(firstMetadata["assignees"]) != "Ada Lovelace" {
		t.Fatalf("first task assignees = %q", assigneesDisplay(firstMetadata["assignees"]))
	}
	_, secondMetadata, err := ws.FindTask("82")
	if err != nil {
		t.Fatal(err)
	}
	if assigneesDisplay(secondMetadata["assignees"]) != "Grace Hopper" {
		t.Fatalf("second task assignees = %q, want replacement of Ada", assigneesDisplay(secondMetadata["assignees"]))
	}
	if len(interaction.initialState) != 2 || !interaction.initialState[1]["user-ada"] {
		t.Fatalf("selector was not prefilled from existing assignees: %#v", interaction.initialState)
	}
	if !strings.Contains(output.String(), "Ada Lovelace") || !strings.Contains(output.String(), "Grace Hopper") || !strings.Contains(output.String(), "ASSIGNEE LOAD") {
		t.Fatalf("assignment summary missing names or effort:\n%s", output.String())
	}
}

func TestFilteredAssignRequiresTerminal(t *testing.T) {
	ws, _ := makeSyncedTask(t, listTask("assign-nonterminal", 91, "toDo", "a", "s", "Task", "feature"))
	if err := assignTask(ws, []string{"--status", "toDo"}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("filtered non-terminal assign error = %v", err)
	}
}
