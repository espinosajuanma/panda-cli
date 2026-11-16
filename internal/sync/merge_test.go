package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func TestMergeForPushIncludesStatusEdit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := map[string]any{"status": "new", "description": ""}
	local := map[string]any{"status": "inProgress", "description": ""}
	remote := map[string]any{"status": "new", "description": ""}

	payload, changed, err := MergeForPush(local, remote, baseline, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || jsonutil.StringField(payload, "status") != "inProgress" {
		t.Fatalf("merged payload = %#v, changed=%v", payload, changed)
	}
}

func TestTaskConflictsOnlyWhenBothSidesChangedSameField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("same description"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := map[string]any{
		"title":       "Original title",
		"status":      "new",
		"description": "<p>same description</p>",
	}
	local := map[string]any{"title": "Local title", "status": "new"}
	remote := map[string]any{
		"title":       "Original title",
		"status":      "inProgress",
		"description": "<p>same description</p>",
	}
	if conflicts := TaskConflicts(local, baseline, remote, dir); len(conflicts) != 0 {
		t.Fatalf("independent edits should merge, got conflicts %v", conflicts)
	}
	remote["title"] = "Remote title"
	conflicts := TaskConflicts(local, baseline, remote, dir)
	if !reflect.DeepEqual(conflicts, []string{"title"}) {
		t.Fatalf("got conflicts %v, want [title]", conflicts)
	}
}

func TestAdvanceBaselineKeepsOldValuesOnlyForConflicts(t *testing.T) {
	baseline := map[string]any{
		"title":  "old title",
		"status": "new",
	}
	remote := map[string]any{
		"id":     "task-1",
		"title":  "remote title",
		"status": "inProgress",
	}
	advanced := AdvanceBaseline(baseline, remote, []string{"title"}, false)
	if advanced["title"] != "old title" {
		t.Fatalf("conflicted baseline title = %v", advanced["title"])
	}
	if advanced["status"] != "inProgress" {
		t.Fatalf("non-conflicted remote update was not advanced: %v", advanced["status"])
	}
	if advanced["id"] != "task-1" {
		t.Fatalf("remote-only field was not advanced: %#v", advanced)
	}
}

func TestMergeForPushPreservesFreshRemoteFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte("original body"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := map[string]any{"title": "Edited title", "status": "new"}
	baseline := map[string]any{
		"title":       "Old title",
		"status":      "new",
		"description": "<p>original body</p>",
	}
	remote := map[string]any{
		"id":          "task-9",
		"title":       "Old title",
		"status":      "inProgress",
		"description": "<p>original body</p>",
		"package":     map[string]any{"id": "package-1"},
	}
	merged, changed, err := MergeForPush(local, remote, baseline, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected local title update")
	}
	if merged["title"] != "Edited title" || merged["status"] != "inProgress" {
		t.Fatalf("unexpected merged task: %#v", merged)
	}
	if !reflect.DeepEqual(merged["package"], remote["package"]) {
		t.Fatalf("fresh remote fields were lost: %#v", merged)
	}
}
