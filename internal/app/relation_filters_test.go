package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestListFiltersByAssigneeAndReviewerWithORAndAND(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	writeMembersForFilterTest(t, ws, []map[string]any{
		{"user": map[string]any{"id": "user-ada", "label": "Ada Lovelace", "email": "ada@example.com"}, "role": "developer"},
		{"user": map[string]any{"id": "user-grace", "label": "Grace Hopper", "email": "grace@example.com"}, "role": "qa"},
	})

	taskA := listTask("filter-a", 101, "new", "a", "s", "Ada and Grace", "feature")
	taskA["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	taskA["reviewers"] = []any{map[string]any{"id": "user-grace", "label": "Grace Hopper"}}
	taskB := listTask("filter-b", 102, "new", "b", "m", "Grace and Ada", "feature")
	taskB["assignees"] = []any{map[string]any{"id": "user-grace", "label": "Grace Hopper"}}
	taskB["reviewers"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	taskC := listTask("filter-c", 103, "new", "c", "l", "Unassigned task", "bug")
	taskD := listTask("filter-d", 104, "new", "d", "xl", "Ada only", "feature")
	taskD["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	for _, fixture := range []map[string]any{taskA, taskB, taskC, taskD} {
		addSyncedTask(t, ws, fixture)
	}

	tests := []struct {
		name      string
		args      []string
		email     string
		wantTasks []string
		omitTasks []string
		wantCols  []string
	}{
		{
			name:      "assignee values are a union",
			args:      []string{"--assignee", "Ada Lovelace,grace@example.com"},
			wantTasks: []string{"Ada and Grace", "Grace and Ada", "Ada only"},
			omitTasks: []string{"Unassigned task"},
			wantCols:  []string{"ASSIGNEES", "Ada Lovelace", "Grace Hopper"},
		},
		{
			name:      "assignee and reviewer filters intersect",
			args:      []string{"--assignee", "user-ada", "--reviewer", "Grace Hopper"},
			wantTasks: []string{"Ada and Grace"},
			omitTasks: []string{"Grace and Ada", "Unassigned task", "Ada only"},
			wantCols:  []string{"ASSIGNEES", "REVIEWERS"},
		},
		{
			name:      "none matches empty relations",
			args:      []string{"--assignee", "none"},
			wantTasks: []string{"Unassigned task"},
			omitTasks: []string{"Ada and Grace", "Grace and Ada", "Ada only"},
			wantCols:  []string{"ASSIGNEES"},
		},
		{
			name:      "reviewer none and me",
			args:      []string{"--reviewer", "none", "--assignee", "me"},
			email:     "ada@example.com",
			wantTasks: []string{"Ada only"},
			omitTasks: []string{"Ada and Grace", "Grace and Ada", "Unassigned task"},
			wantCols:  []string{"ASSIGNEES", "REVIEWERS"},
		},
		{
			name:      "me resolves reviewer by saved email",
			args:      []string{"--reviewer", "@me"},
			email:     "grace@example.com",
			wantTasks: []string{"Ada and Grace"},
			omitTasks: []string{"Grace and Ada", "Unassigned task", "Ada only"},
			wantCols:  []string{"REVIEWERS"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := listTasksForUser(ws, test.args, test.email, &output); err != nil {
				t.Fatal(err)
			}
			for _, want := range test.wantTasks {
				if !strings.Contains(output.String(), want) {
					t.Errorf("filtered output missing task %q:\n%s", want, output.String())
				}
			}
			for _, omit := range test.omitTasks {
				if strings.Contains(output.String(), omit) {
					t.Errorf("filtered output unexpectedly contains %q:\n%s", omit, output.String())
				}
			}
			for _, column := range test.wantCols {
				if !strings.Contains(output.String(), column) {
					t.Errorf("filtered output missing column/value %q:\n%s", column, output.String())
				}
			}
		})
	}
}

func TestListRelationshipFiltersApplyToJSONOutput(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	writeMembersForFilterTest(t, ws, []map[string]any{{"user": map[string]any{"id": "user-ada", "label": "Ada Lovelace", "email": "ada@example.com"}, "role": "developer"}})
	assigned := listTask("json-filter-assigned", 111, "new", "a", "s", "Assigned", "feature")
	assigned["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	unassigned := listTask("json-filter-unassigned", 112, "new", "b", "m", "Unassigned", "bug")
	addSyncedTask(t, ws, assigned)
	addSyncedTask(t, ws, unassigned)

	var output strings.Builder
	if err := listTasksForUser(ws, []string{"--assignee=me", "--json"}, "ada@example.com", &output); err != nil {
		t.Fatal(err)
	}
	var document listJSONDocument
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Groups) != 1 || len(document.Groups[0].Tasks) != 1 || stringField(document.Groups[0].Tasks[0].Task, "id") != "json-filter-assigned" {
		t.Fatalf("JSON relationship filter result = %#v", document)
	}
}

func TestListRelationshipFilterFallbackAndValidation(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	assigned := listTask("fallback-assigned", 121, "new", "a", "s", "Fallback assigned", "feature")
	assigned["assignees"] = []any{map[string]any{"id": "user-ada", "label": "Ada Lovelace"}}
	addSyncedTask(t, ws, assigned)

	var output strings.Builder
	if err := listTasks(ws, []string{"--assignee", "Ada Lovelace"}, &output); err != nil {
		t.Fatalf("name matching without members file: %v", err)
	}
	if !strings.Contains(output.String(), "Fallback assigned") {
		t.Fatalf("fallback name match did not include task:\n%s", output.String())
	}
	output.Reset()
	if err := listTasks(ws, []string{"--assignee", "user-ada"}, &output); err != nil {
		t.Fatalf("ID matching without members file: %v", err)
	}
	if !strings.Contains(output.String(), "Fallback assigned") {
		t.Fatalf("fallback ID match did not include task:\n%s", output.String())
	}
	output.Reset()
	if err := listTasks(ws, []string{"--assignee", "me"}, &output); err == nil || !strings.Contains(err.Error(), "saved login email") {
		t.Fatalf("me without saved email error = %v", err)
	}
	if err := listTasksForUser(ws, []string{"--assignee", "me"}, "ada@example.com", &output); err == nil || !strings.Contains(err.Error(), "synced project members") {
		t.Fatalf("me without members error = %v", err)
	}
	if _, err := parseListOptions([]string{"--assignee", ""}); err == nil {
		t.Fatal("empty assignee filter was accepted")
	}
	writeMembersForFilterTest(t, ws, []map[string]any{
		{"user": map[string]any{"id": "user-ada", "label": "Ada", "email": "ada@example.com"}, "role": "developer"},
		{"user": map[string]any{"id": "user-other", "label": "Ada", "email": "other@example.com"}, "role": "qa"},
	})
	if err := listTasks(ws, []string{"--assignee", "none", "--assignee", "Ada"}, &output); err == nil || !strings.Contains(err.Error(), "must be used alone") {
		t.Fatalf("none combined with a member error = %v", err)
	}
	if err := listTasks(ws, []string{"--assignee", "Ada"}, &output); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous member selector error = %v", err)
	}
}

func writeMembersForFilterTest(t *testing.T, ws *workspace.Workspace, members []map[string]any) {
	t.Helper()
	data, err := json.Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Root, ".panda", "members.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRelationFilterContainsResolvedAliases(t *testing.T) {
	filter, err := resolveRelationFilter([]map[string]any{{"user": map[string]any{"id": "user-1", "label": "Ada", "email": "ada@example.com"}}}, true, []string{"ada@example.com"}, "", "assignee")
	if err != nil {
		t.Fatal(err)
	}
	if !filter.matches([]any{map[string]any{"id": "user-1", "label": "Ada"}}) || !reflect.DeepEqual(filter.names, map[string]bool{"ada": true, "ada@example.com": true, "user-1": true}) {
		t.Fatalf("resolved filter aliases = %#v", filter)
	}
}

func TestParseListOptionsAcceptsRepeatedAndCommaSeparatedRelationshipValues(t *testing.T) {
	options, err := parseListOptions([]string{
		"--assignee", "ada@example.com,grace@example.com",
		"--assignee=Ada Lovelace", "--reviewer", "me,none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(options.assignees, []string{"ada@example.com", "grace@example.com", "Ada Lovelace"}) || !reflect.DeepEqual(options.reviewers, []string{"me", "none"}) {
		t.Fatalf("parsed relationship selectors = assignees %v, reviewers %v", options.assignees, options.reviewers)
	}
}
