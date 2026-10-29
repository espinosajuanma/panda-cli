package catalog

import (
	"reflect"
	"testing"
)

func TestMergeRecordsKeepsIndependentChangesAndReportsOverlap(t *testing.T) {
	baseline := []map[string]any{{"id": "label-1", "name": "bug", "description": "old"}}
	local := []map[string]any{{"id": "label-1", "name": "defect", "description": "old"}}
	remote := []map[string]any{{"id": "label-1", "name": "bug", "description": "updated remotely"}}
	merged, advanced, conflicts := MergeRecords(Labels, local, baseline, remote)
	if len(conflicts) != 0 {
		t.Fatalf("independent changes unexpectedly conflicted: %v", conflicts)
	}
	if merged[0]["name"] != "defect" || merged[0]["description"] != "updated remotely" {
		t.Fatalf("unexpected merged record: %#v", merged[0])
	}
	if advanced[0]["name"] != "bug" || advanced[0]["description"] != "updated remotely" {
		t.Fatalf("unexpected advanced baseline: %#v", advanced[0])
	}

	remote[0]["name"] = "issue"
	_, _, conflicts = MergeRecords(Labels, local, baseline, remote)
	if !reflect.DeepEqual(conflicts, []string{`label "bug": name`}) {
		t.Fatalf("conflicts = %v, want overlapping name conflict", conflicts)
	}
}
