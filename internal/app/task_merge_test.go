package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeForPushIncludesStatusEdit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "description.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := map[string]any{"status": "new", "description": ""}
	local := map[string]any{"status": "inProgress", "description": ""}
	remote := map[string]any{"status": "new", "description": ""}

	payload, changed, err := mergeForPush(local, remote, baseline, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || stringField(payload, "status") != "inProgress" {
		t.Fatalf("merged payload = %#v, changed=%v", payload, changed)
	}
}
