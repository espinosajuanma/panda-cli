package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func setTestConfigHome(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
}

func TestDoctorHealthyWorkspaceIsReadOnly(t *testing.T) {
	ws, _ := makeDoctorWorkspace(t)
	var output strings.Builder
	if err := Run(nil, &output); err != nil {
		t.Fatalf("doctor healthy workspace: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "0 failure(s), 0 warning(s)") {
		t.Fatalf("healthy workspace findings:\n%s", output.String())
	}
	if _, err := os.Stat(filepath.Join(ws.Root, ".panda", "workspace.lock")); !os.IsNotExist(err) {
		t.Fatalf("doctor created a workspace lock: %v", err)
	}
}

func TestDoctorJSONReportsLocalFailuresAndDoesNotLeakToken(t *testing.T) {
	ws, taskDir := makeDoctorWorkspace(t)
	if err := os.Remove(ws.StatePathForKey("doctor-task")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "description.md"), []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ws.StatePathForKey("broken-state"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	err := Run([]string{"--json"}, &output)
	if err == nil {
		t.Fatal("doctor succeeded despite invalid local workspace data")
	}
	var report Report
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatalf("invalid doctor JSON %q: %v", output.String(), err)
	}
	if report.SchemaVersion != 1 || report.OK || report.Summary.Failures < 3 {
		t.Fatalf("doctor JSON summary = %#v", report)
	}
	codes := make(map[string]bool)
	for _, finding := range report.Findings {
		codes[finding.Code] = true
	}
	for _, code := range []string{"task.missing_baseline", "task.description.invalid_utf8", "state.invalid"} {
		if !codes[code] {
			t.Errorf("doctor findings lack %q: %#v", code, report.Findings)
		}
	}
	if strings.Contains(output.String(), "doctor-secret-token") {
		t.Fatal("doctor output included the saved token")
	}
}

func TestDoctorWithoutProjectWarnsWithoutCreatingWorkspace(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("PANDA_API_URL", "")
	cfg := &config.Config{APIURL: config.DefaultAPIURL, Token: "doctor-token"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	workspaceDir, err := config.WorkspaceDirectory()
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := Run([]string{"--json"}, &output); err != nil {
		t.Fatalf("warnings should not fail doctor: %v", err)
	}
	var report Report
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Summary.Warnings == 0 {
		t.Fatalf("no-project report = %#v", report)
	}
	if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
		t.Fatalf("doctor created the workspace directory: %v", err)
	}
}

func TestDoctorReportsHeldWorkspaceLockAsWarning(t *testing.T) {
	ws, _ := makeDoctorWorkspace(t)
	lock, err := ws.LockWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var output strings.Builder
	if err := Run([]string{"--json"}, &output); err != nil {
		t.Fatalf("held lock should be a warning: %v", err)
	}
	var report Report
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Failures != 0 || !hasCode(report, "workspace.lock.held") {
		t.Fatalf("held lock report = %#v", report)
	}
}

func TestDoctorWarningsDoNotFailForUnknownFieldsOrOrphanedStagedData(t *testing.T) {
	ws, taskDir := makeDoctorWorkspace(t)
	metadataPath := filepath.Join(taskDir, "metadata.json")
	metadata, err := workspace.ReadMap(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata["status"] = "customStatus"
	if err := workspace.WriteMap(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}
	state, err := ws.ReadState("doctor-task")
	if err != nil {
		t.Fatal(err)
	}
	state.StagedNotes = []string{"missing-note.md"}
	state.StagedFiles = []workspace.StagedFile{{Path: filepath.Join(t.TempDir(), "missing-upload.png"), Name: "missing-upload.png"}}
	if err := ws.WriteState("doctor-task", state); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteState("orphan-state", workspace.State{}); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := Run([]string{"--json"}, &output); err != nil {
		t.Fatalf("warnings should not fail doctor: %v", err)
	}
	var report Report
	if err := json.Unmarshal([]byte(output.String()), &report); err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Summary.Failures != 0 || report.Summary.Warnings < 4 {
		t.Fatalf("warning-only report = %#v", report)
	}
	for _, code := range []string{"task.field.unknown", "task.staged_note.missing", "task.staged_file.missing", "state.orphan"} {
		if !hasCode(report, code) {
			t.Errorf("doctor warnings lack %q: %#v", code, report.Findings)
		}
	}
}

func makeDoctorWorkspace(t *testing.T) (*workspace.Workspace, string) {
	t.Helper()
	setTestConfigHome(t)
	t.Setenv("PANDA_API_URL", "")
	cfg := &config.Config{
		APIURL: config.DefaultAPIURL, Token: "doctor-secret-token", Email: "ada@example.com",
		Project: &config.Project{ID: "project-doctor", Name: "Doctor Project"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	remote := map[string]any{
		"id": "doctor-task", "number": float64(901), "title": "Doctor health check", "type": "feature",
		"priority": "medium", "complexity": "s", "status": "new", "description": "<p>Healthy</p>",
		"project": map[string]any{"id": cfg.Project.ID, "label": cfg.Project.Name},
		"labels":  []any{}, "assignees": []any{}, "reviewers": []any{}, "notes": []any{},
	}
	dir := ws.TaskPath(workspace.DirectoryName(remote))
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte("Healthy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteState("doctor-task", workspace.State{Remote: remote}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []catdomain.Kind{catdomain.Labels, catdomain.Packages} {
		if err := ws.WriteCatalog(kind, []map[string]any{}); err != nil {
			t.Fatal(err)
		}
		if err := ws.WriteCatalogBaseline(kind, []map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.WriteMembers([]map[string]any{{"user": map[string]any{"id": "user-ada", "label": "Ada Lovelace", "email": "ada@example.com"}, "role": "developer"}}); err != nil {
		t.Fatal(err)
	}
	return ws, dir
}

func hasCode(report Report, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
