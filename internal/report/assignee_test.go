package report

import "testing"

func TestSummarizeAssigneeEffortSortsByStoryPoints(t *testing.T) {
	tasks := []map[string]any{
		{"complexity": "m", "assignees": []any{map[string]any{"id": "u1", "label": "Ada"}}},
		{"complexity": "l", "assignees": []any{map[string]any{"id": "u2", "label": "Grace"}}},
		{"complexity": "s", "assignees": []any{}},
	}
	totals, shared := SummarizeAssigneeEffort(tasks)
	if shared != 0 {
		t.Fatalf("shared = %d, want 0", shared)
	}
	if len(totals) != 3 {
		t.Fatalf("totals = %#v", totals)
	}
	if totals[0].Assignee != "Grace" || totals[0].StoryPoints != 5 {
		t.Fatalf("first total = %#v, want Grace with 5 SP", totals[0])
	}
	if totals[1].Assignee != "Ada" || totals[1].StoryPoints != 3 {
		t.Fatalf("second total = %#v, want Ada with 3 SP", totals[1])
	}
	if totals[2].Assignee != "Unassigned" || totals[2].StoryPoints != 2 {
		t.Fatalf("third total = %#v, want Unassigned with 2 SP", totals[2])
	}
}

func TestSummarizeAssigneeEffortCountsSharedTasks(t *testing.T) {
	tasks := []map[string]any{
		{"complexity": "m", "assignees": []any{
			map[string]any{"id": "u1", "label": "Ada"},
			map[string]any{"id": "u2", "label": "Grace"},
		}},
	}
	totals, shared := SummarizeAssigneeEffort(tasks)
	if shared != 1 {
		t.Fatalf("shared = %d, want 1", shared)
	}
	for _, total := range totals {
		if total.Tasks != 1 || total.StoryPoints != 3 {
			t.Fatalf("total = %#v, want shared task counted for each assignee", total)
		}
	}
}
