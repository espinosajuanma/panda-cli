package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func setTestConfigHome(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
}

func listTask(id string, number float64, status, rank string, complexity any, title, taskType string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "title": title, "type": taskType,
		"priority": "medium", "complexity": complexity, "status": status, "rank": rank,
		"description": "", "project": map[string]any{"id": "project-1"},
		"labels": []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
}

func makeSyncedTask(t *testing.T, remote map[string]any) (*workspace.Workspace, string) {
	t.Helper()
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := addSyncedTask(t, ws, remote)
	return ws, dir
}

func addSyncedTask(t *testing.T, ws *workspace.Workspace, remote map[string]any) string {
	t.Helper()
	dir := ws.TaskPath(workspace.DirectoryName(remote))
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		t.Fatal(err)
	}
	markdown, err := content.HTMLToMarkdown(stringField(remote, "description"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeReleaseNotesToLocal(dir, remote); err != nil {
		t.Fatal(err)
	}
	id := stringField(remote, "id")
	if err := ws.WriteState(id, workspace.State{Remote: cloneMap(remote)}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func marshalForTest(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
