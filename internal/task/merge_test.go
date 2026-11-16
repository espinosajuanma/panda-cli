package task

import (
	"reflect"
	"testing"
)

func TestMergeFieldsAppliesIndependentLocalChanges(t *testing.T) {
	local := FromMap(map[string]any{"title": "Local title", "priority": "medium", "custom": "local"})
	remote := FromMap(map[string]any{"title": "Remote title", "priority": "medium", "custom": "remote", "unknown": true})
	baseline := FromMap(map[string]any{"title": "Original title", "priority": "medium", "custom": "baseline"})

	merged, changed := MergeFields(local, remote, baseline)
	if !changed {
		t.Fatal("expected changed merge")
	}
	if merged.StringField("title") != "Local title" {
		t.Fatalf("title = %q, want local title", merged.StringField("title"))
	}
	if merged.Field("custom") != "remote" {
		t.Fatalf("unchanged field should come from remote: %v", merged.Field("custom"))
	}
	if merged.Field("unknown") != true {
		t.Fatalf("unknown remote field was dropped: %#v", merged.Map())
	}
	if remote.StringField("title") != "Remote title" {
		t.Fatal("merge mutated the remote input")
	}
}

func TestMergeFieldsReportsNoChangeForRemoteOnlyEdits(t *testing.T) {
	local := FromMap(map[string]any{"title": "Original title"})
	remote := FromMap(map[string]any{"title": "Remote title"})
	baseline := FromMap(map[string]any{"title": "Original title"})

	merged, changed := MergeFields(local, remote, baseline)
	if changed {
		t.Fatal("remote-only edit should not be pushed")
	}
	if merged.StringField("title") != "Remote title" {
		t.Fatalf("title = %q, want remote title", merged.StringField("title"))
	}
}

func TestMetadataConflictsRequiresBothSidesToDiverge(t *testing.T) {
	baseline := FromMap(map[string]any{"title": "Original", "priority": "medium", "status": "toDo"})
	tests := []struct {
		name   string
		local  map[string]any
		remote map[string]any
		want   []string
	}{
		{
			name:   "local only",
			local:  map[string]any{"title": "Local", "priority": "medium", "status": "toDo"},
			remote: map[string]any{"title": "Original", "priority": "medium", "status": "toDo"},
			want:   nil,
		},
		{
			name:   "remote only",
			local:  map[string]any{"title": "Original", "priority": "medium", "status": "toDo"},
			remote: map[string]any{"title": "Remote", "priority": "medium", "status": "toDo"},
			want:   nil,
		},
		{
			name:   "same change",
			local:  map[string]any{"title": "Same", "priority": "medium", "status": "toDo"},
			remote: map[string]any{"title": "Same", "priority": "medium", "status": "toDo"},
			want:   nil,
		},
		{
			name:   "diverged",
			local:  map[string]any{"title": "Local", "priority": "high", "status": "toDo"},
			remote: map[string]any{"title": "Remote", "priority": "medium", "status": "toDo"},
			want:   []string{"title"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MetadataConflicts(FromMap(test.local), baseline, FromMap(test.remote))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("conflicts = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAdvanceBaselineKeepsConflictsAndAdvancesCleanFields(t *testing.T) {
	baseline := FromMap(map[string]any{"title": "Original", "priority": "medium", "description": "<p>Old</p>", "removed": "keep"})
	remote := FromMap(map[string]any{"title": "Remote", "priority": "high", "description": "<p>New</p>"})

	advanced := AdvanceBaseline(baseline, remote, []string{"title", "removed"}, true)
	if advanced.StringField("title") != "Original" {
		t.Fatalf("conflicting title = %q, want baseline", advanced.StringField("title"))
	}
	if advanced.Field("removed") != "keep" {
		t.Fatalf("missing baseline field should be retained on conflict: %v", advanced.Field("removed"))
	}
	if advanced.StringField("priority") != "high" {
		t.Fatalf("non-conflicting field = %q, want remote", advanced.StringField("priority"))
	}
	if advanced.StringField("description") != "<p>Old</p>" {
		t.Fatalf("description conflict baseline = %q", advanced.StringField("description"))
	}
}

func TestSameVersionRequiresStableVersionAndRelationships(t *testing.T) {
	baseline := FromMap(map[string]any{"version": float64(2), "labels": []any{"a"}, "package": nil, "files": []any{}})
	same := FromMap(map[string]any{"version": float64(2), "labels": []any{"a"}, "package": nil, "files": []any{}})
	if !SameVersion(same, baseline) {
		t.Fatal("expected same version")
	}
	for name, remote := range map[string]map[string]any{
		"version": {"version": float64(3), "labels": []any{"a"}, "package": nil, "files": []any{}},
		"labels":  {"version": float64(2), "labels": []any{"b"}, "package": nil, "files": []any{}},
		"package": {"version": float64(2), "labels": []any{"a"}, "package": map[string]any{"id": "p"}, "files": []any{}},
		"files":   {"version": float64(2), "labels": []any{"a"}, "package": nil, "files": []any{"f"}},
		"missing": {"labels": []any{"a"}, "package": nil, "files": []any{}},
	} {
		if SameVersion(FromMap(remote), baseline) {
			t.Errorf("%s change was not detected", name)
		}
	}
}
