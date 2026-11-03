package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type doctorOptions struct {
	json bool
}

type doctorReport struct {
	SchemaVersion int             `json:"schemaVersion"`
	OK            bool            `json:"ok"`
	Summary       doctorSummary   `json:"summary"`
	Findings      []doctorFinding `json:"findings"`
}

type doctorSummary struct {
	Checks   int `json:"checks"`
	Failures int `json:"failures"`
	Warnings int `json:"warnings"`
}

type doctorFinding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type doctorBuilder struct {
	report doctorReport
}

func doctorCommand(args []string, out io.Writer) error {
	options, err := parseDoctorOptions(args)
	if err != nil {
		return err
	}
	report := inspectLocalWorkspace()
	if options.json {
		if err := renderDoctorJSON(report, out); err != nil {
			return err
		}
	} else {
		renderDoctorText(report, out)
	}
	if report.Summary.Failures > 0 {
		return fmt.Errorf("doctor found %d failure(s)", report.Summary.Failures)
	}
	return nil
}

func parseDoctorOptions(args []string) (doctorOptions, error) {
	var options doctorOptions
	for _, arg := range args {
		if arg == "--json" {
			options.json = true
			continue
		}
		return doctorOptions{}, errors.New("usage: panda doctor [--json]")
	}
	return options, nil
}

func inspectLocalWorkspace() doctorReport {
	builder := doctorBuilder{report: doctorReport{
		SchemaVersion: 1,
		Findings:      make([]doctorFinding, 0),
	}}

	configDir, err := config.Directory()
	if err != nil {
		builder.fail("config.directory", "", fmt.Sprintf("resolve configuration directory: %v", err))
		return builder.finish()
	}
	configPath := filepath.Join(configDir, "config.json")
	if info, statErr := os.Stat(configDir); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			builder.warn("config.directory.missing", configDir, "configuration directory does not exist yet")
		} else {
			builder.fail("config.directory.unreadable", configDir, fmt.Sprintf("inspect configuration directory: %v", statErr))
		}
	} else if !info.IsDir() {
		builder.fail("config.directory.invalid", configDir, "configuration path is not a directory")
	} else {
		builder.ok("config.directory", configDir, "configuration directory is available")
		builder.checkPrivateMode("config.directory.permissions", configDir, info, true)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		builder.fail("config.invalid", configPath, fmt.Sprintf("load configuration: %v", loadErr))
		cfg = readDoctorConfigFallback(configPath)
	} else {
		builder.ok("config.valid", configPath, "configuration parses and its API URL is valid")
	}
	if info, statErr := os.Stat(configPath); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			builder.warn("config.file.missing", configPath, "configuration file does not exist yet")
		} else {
			builder.fail("config.file.unreadable", configPath, fmt.Sprintf("inspect configuration file: %v", statErr))
		}
	} else if !info.Mode().IsRegular() {
		builder.fail("config.file.invalid", configPath, "configuration path is not a regular file")
	} else {
		builder.checkPrivateMode("config.file.permissions", configPath, info, false)
	}
	if cfg == nil {
		builder.warn("config.project.unavailable", "", "project and session checks were skipped because configuration could not be read")
		return builder.finish()
	}
	if cfg.Token == "" {
		builder.warn("config.session.missing", "", "no Solutions login token is saved; remote commands require `panda login`")
	} else {
		builder.ok("config.session.present", "", "a Solutions login token is saved")
	}
	if cfg.Project == nil {
		builder.warn("config.project.missing", "", "no project is selected; run `panda config set project <name>`")
		return builder.finish()
	}
	if strings.TrimSpace(cfg.Project.ID) == "" || strings.TrimSpace(cfg.Project.Name) == "" {
		builder.fail("config.project.invalid", "", "selected project is missing its ID or name")
		return builder.finish()
	}
	builder.ok("config.project.selected", "", "a project is selected")

	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		builder.fail("workspace.path.invalid", "", fmt.Sprintf("resolve selected workspace: %v", err))
		return builder.finish()
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			builder.warn("workspace.missing", root, "project workspace does not exist yet; run `panda pull` to initialize it")
		} else {
			builder.fail("workspace.unreadable", root, fmt.Sprintf("inspect project workspace: %v", err))
		}
		return builder.finish()
	}
	if !rootInfo.IsDir() {
		builder.fail("workspace.invalid", root, "project workspace path is not a directory")
		return builder.finish()
	}
	builder.ok("workspace.available", root, "project workspace is available")
	builder.checkPrivateMode("workspace.permissions", root, rootInfo, true)
	ws := workspace.New(root)

	directoryPaths := []struct {
		name string
		code string
	}{
		{filepath.Join(root, ".panda"), "workspace.panda_directory"},
		{filepath.Join(root, ".panda", "state"), "workspace.state_directory"},
		{ws.TasksRoot(), "workspace.tasks_directory"},
	}
	for _, current := range directoryPaths {
		info, statErr := os.Stat(current.name)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				builder.warn(current.code+".missing", doctorRelativePath(root, current.name), "workspace directory is missing")
			} else {
				builder.fail(current.code+".unreadable", doctorRelativePath(root, current.name), fmt.Sprintf("inspect directory: %v", statErr))
			}
			continue
		}
		if !info.IsDir() {
			builder.fail(current.code+".invalid", doctorRelativePath(root, current.name), "workspace path is not a directory")
			continue
		}
		builder.ok(current.code, doctorRelativePath(root, current.name), "directory is available")
		builder.checkPrivateMode(current.code+".permissions", doctorRelativePath(root, current.name), info, true)
	}

	if _, pandaErr := os.Stat(filepath.Join(root, ".panda")); pandaErr == nil {
		exists, held, probeErr := ws.ProbeLock()
		lockPath := filepath.Join(root, ".panda", "workspace.lock")
		switch {
		case probeErr != nil:
			builder.warn("workspace.lock.unknown", ".panda/workspace.lock", fmt.Sprintf("could not inspect workspace lock: %v", probeErr))
		case held:
			builder.warn("workspace.lock.held", ".panda/workspace.lock", "a Panda process currently holds the workspace lock")
		case exists:
			builder.ok("workspace.lock.available", ".panda/workspace.lock", "workspace lock is not currently held")
		default:
			builder.ok("workspace.lock.absent", ".panda/workspace.lock", "no workspace lock file exists")
		}
		if exists {
			if info, statErr := os.Stat(lockPath); statErr == nil {
				builder.checkPrivateMode("workspace.lock.permissions", ".panda/workspace.lock", info, false)
			}
		}
	}

	states, invalidStates, stateKeys := inspectWorkspaceStates(&builder, ws, root)
	inspectProjectCatalogs(&builder, ws, root, states, invalidStates)
	inspectProjectMembers(&builder, ws, root)
	expectedStateKeys := inspectTaskDirectories(&builder, ws, root, states, invalidStates)
	inspectOrphanStates(&builder, root, stateKeys, expectedStateKeys)
	inspectTemporaryFiles(&builder, root)
	return builder.finish()
}

func readDoctorConfigFallback(path string) *config.Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func inspectWorkspaceStates(builder *doctorBuilder, ws *workspace.Workspace, root string) (map[string]workspace.State, map[string]bool, []string) {
	states := make(map[string]workspace.State)
	invalid := make(map[string]bool)
	keys, err := ws.StateKeys()
	if err != nil {
		builder.fail("state.directory.invalid", ".panda/state", fmt.Sprintf("read state filenames: %v", err))
		return states, invalid, nil
	}
	for _, key := range keys {
		path := ws.StatePathForKey(key)
		relative := doctorRelativePath(root, path)
		if info, statErr := os.Stat(path); statErr == nil {
			builder.checkPrivateMode("state.permissions", relative, info, false)
		}
		state, readErr := ws.ReadState(key)
		if readErr != nil {
			builder.fail("state.invalid", relative, fmt.Sprintf("read sync state: %v", readErr))
			invalid[key] = true
			continue
		}
		states[key] = state
	}
	builder.ok("state.scan", ".panda/state", fmt.Sprintf("inspected %d state file(s)", len(keys)))
	return states, invalid, keys
}

func inspectProjectCatalogs(builder *doctorBuilder, ws *workspace.Workspace, root string, states map[string]workspace.State, invalid map[string]bool) {
	for _, kind := range []catalogKind{labelCatalog, packageCatalog} {
		path := ws.CatalogPath(string(kind))
		relative := doctorRelativePath(root, path)
		if info, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				builder.warn("catalog.missing", relative, fmt.Sprintf("project %s catalog is not present; run `panda pull`", kind))
			} else {
				builder.fail("catalog.unreadable", relative, fmt.Sprintf("inspect project %s catalog: %v", kind, err))
			}
		} else {
			builder.checkPrivateMode("catalog.permissions", relative, info, false)
			if _, err := readCatalog(ws, kind); err != nil {
				builder.fail("catalog.invalid", relative, fmt.Sprintf("parse project %s catalog: %v", kind, err))
			} else {
				builder.ok("catalog.valid", relative, fmt.Sprintf("project %s catalog parses", kind))
			}
		}
		key := ws.CatalogStateKey(string(kind))
		state, ok := states[key]
		if !ok {
			if !invalid[key] {
				if _, err := os.Stat(path); err == nil {
					builder.warn("catalog.baseline.missing", ".panda/state", fmt.Sprintf("project %s catalog has no sync baseline", kind))
				}
			}
			continue
		}
		if state.Remote == nil {
			builder.fail("catalog.baseline.invalid", ws.StatePathForKey(key), fmt.Sprintf("project %s catalog baseline is empty", kind))
			continue
		}
		if _, ok := state.Remote["records"].([]any); !ok {
			builder.fail("catalog.baseline.invalid", ws.StatePathForKey(key), fmt.Sprintf("project %s catalog baseline has no records array", kind))
		}
	}
}

func inspectProjectMembers(builder *doctorBuilder, ws *workspace.Workspace, root string) {
	members, available, err := projectMembersIfAvailable(ws)
	path := ".panda/members.json"
	if err != nil {
		builder.fail("members.invalid", path, fmt.Sprintf("read project members: %v", err))
		return
	}
	if !available {
		builder.warn("members.missing", path, "project members are not synced; run `panda pull` to enable member-based filters and assignment")
		return
	}
	if info, err := os.Stat(filepath.Join(root, path)); err == nil {
		builder.checkPrivateMode("members.permissions", path, info, false)
	}
	builder.ok("members.valid", path, fmt.Sprintf("read %d project member record(s)", len(members)))
}

func inspectTaskDirectories(builder *doctorBuilder, ws *workspace.Workspace, root string, states map[string]workspace.State, invalidStates map[string]bool) map[string]bool {
	expectedStateKeys := map[string]bool{
		ws.CatalogStateKey(string(labelCatalog)):   true,
		ws.CatalogStateKey(string(packageCatalog)): true,
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		builder.fail("tasks.directory.invalid", "tasks", fmt.Sprintf("list task directories: %v", err))
		return expectedStateKeys
	}
	if len(dirs) == 0 {
		builder.ok("tasks.scan", "tasks", "no local task directories to inspect")
		return expectedStateKeys
	}
	builder.ok("tasks.scan", "tasks", fmt.Sprintf("inspected %d task directory(ies)", len(dirs)))
	seenIDs, seenNumbers := make(map[string]string), make(map[string]string)
	for _, dir := range dirs {
		relDir := doctorRelativePath(root, dir)
		metadataPath := filepath.Join(dir, "metadata.json")
		metadata, err := workspace.ReadMap(metadataPath)
		if err != nil {
			builder.fail("task.metadata.invalid", doctorRelativePath(root, metadataPath), fmt.Sprintf("read task metadata: %v", err))
			continue
		}
		if info, statErr := os.Stat(metadataPath); statErr == nil {
			builder.checkPrivateMode("task.metadata.permissions", doctorRelativePath(root, metadataPath), info, false)
		}
		id, localID := stringField(metadata, "id"), stringField(metadata, "localId")
		if id == "" && localID == "" && !isLocalDraft(dir, metadata) {
			builder.fail("task.identity.missing", relDir, "task metadata has neither a remote ID nor a local draft ID")
		}
		if title := strings.TrimSpace(stringField(metadata, "title")); title == "" {
			builder.warn("task.title.missing", relDir, "task title is empty")
		}
		if id != "" {
			if previous := seenIDs[id]; previous != "" {
				builder.fail("task.id.duplicate", relDir, fmt.Sprintf("remote task ID is also used by %s", previous))
			} else {
				seenIDs[id] = relDir
			}
		}
		if number := stringField(metadata, "number"); number != "" {
			if previous := seenNumbers[number]; previous != "" {
				builder.fail("task.number.duplicate", relDir, fmt.Sprintf("task number %s is also used by %s", number, previous))
			} else {
				seenNumbers[number] = relDir
			}
		}
		inspectTaskFields(builder, metadata, relDir)
		inspectTaskDescription(builder, dir, root)
		inspectTaskNotes(builder, dir, root)

		keys := taskStateKeys(dir, metadata)
		stateFound, stateInvalid := false, false
		for _, key := range keys {
			expectedStateKeys[key] = true
			if invalidStates[key] {
				stateInvalid = true
				continue
			}
			state, ok := states[key]
			if !ok {
				continue
			}
			stateFound = true
			if id != "" {
				if state.LocalCreated {
					builder.fail("task.baseline.local_created", relDir, "synced task has local-created state")
				} else if state.Remote == nil {
					builder.fail("task.baseline.invalid", relDir, "synced task baseline is empty")
				} else if remoteID := stringField(state.Remote, "id"); remoteID != id {
					builder.fail("task.baseline.id_mismatch", relDir, fmt.Sprintf("baseline remote ID %q does not match task ID %q", remoteID, id))
				}
			}
			inspectStagedActions(builder, state, dir, root)
		}
		if id != "" && !stateFound && !stateInvalid {
			builder.fail("task.missing_baseline", relDir, "synced task has no sync baseline; run `panda pull` before push or clean")
		}
	}
	return expectedStateKeys
}

func inspectTaskFields(builder *doctorBuilder, metadata map[string]any, relDir string) {
	for _, field := range []struct {
		name    string
		allowed []string
	}{
		{name: "type", allowed: []string{"feature", "bug", "other", "release"}},
		{name: "priority", allowed: []string{"hotfix", "high", "medium", "low"}},
		{name: "complexity", allowed: task.ComplexityOrder},
		{name: "status", allowed: task.StatusOrder},
	} {
		value := stringField(metadata, field.name)
		if value != "" && !contains(field.allowed, value) {
			builder.warn("task.field.unknown", relDir, fmt.Sprintf("task has unrecognized %s value %q", field.name, value))
		}
	}
}

func inspectTaskDescription(builder *doctorBuilder, dir, root string) {
	path := filepath.Join(dir, "description.md")
	relative := doctorRelativePath(root, path)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			builder.warn("task.description.missing", relative, "task description file is missing")
		} else {
			builder.fail("task.description.unreadable", relative, fmt.Sprintf("read task description: %v", err))
		}
		return
	}
	if !utf8.Valid(data) {
		builder.fail("task.description.invalid_utf8", relative, "task description is not valid UTF-8")
		return
	}
	if info, err := os.Stat(path); err == nil {
		builder.checkPrivateMode("task.description.permissions", relative, info, false)
	}
}

func inspectTaskNotes(builder *doctorBuilder, dir, root string) {
	path := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			builder.warn("task.notes_directory.missing", doctorRelativePath(root, path), "task notes directory is missing")
		} else {
			builder.fail("task.notes_directory.unreadable", doctorRelativePath(root, path), fmt.Sprintf("read task notes directory: %v", err))
		}
		return
	}
	if info, err := os.Stat(path); err == nil {
		builder.checkPrivateMode("task.notes_directory.permissions", doctorRelativePath(root, path), info, true)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		filename := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			builder.fail("task.note.unreadable", doctorRelativePath(root, filename), fmt.Sprintf("read note: %v", err))
		} else if !utf8.Valid(data) {
			builder.fail("task.note.invalid_utf8", doctorRelativePath(root, filename), "task note is not valid UTF-8")
		} else if info, statErr := os.Stat(filename); statErr == nil {
			builder.checkPrivateMode("task.note.permissions", doctorRelativePath(root, filename), info, false)
		}
	}
}

func inspectStagedActions(builder *doctorBuilder, state workspace.State, dir, root string) {
	for _, filename := range state.StagedNotes {
		if filepath.Base(filename) != filename || filename == "." || filename == ".." {
			builder.fail("task.staged_note.path_invalid", doctorRelativePath(root, dir), fmt.Sprintf("staged note filename %q is not a simple filename", filename))
			continue
		}
		path := filepath.Join(dir, "notes", filename)
		if _, err := os.Stat(path); err != nil {
			builder.warn("task.staged_note.missing", doctorRelativePath(root, path), fmt.Sprintf("staged note is unavailable: %v", err))
		}
	}
	for _, staged := range state.StagedFiles {
		if _, err := os.Stat(staged.Path); err != nil {
			builder.warn("task.staged_file.missing", staged.Path, fmt.Sprintf("staged upload source is unavailable: %v", err))
		}
	}
}

func inspectOrphanStates(builder *doctorBuilder, root string, keys []string, expected map[string]bool) {
	for _, key := range keys {
		if expected[key] {
			continue
		}
		builder.warn("state.orphan", doctorRelativePath(root, filepath.Join(root, ".panda", "state")), fmt.Sprintf("state key %q does not correspond to a task or catalog", key))
	}
}

func inspectTemporaryFiles(builder *doctorBuilder, root string) {
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".panda-") && strings.HasSuffix(name, ".tmp") {
			builder.warn("workspace.temporary_file", doctorRelativePath(root, path), "temporary atomic-write file remains in workspace")
		}
		return nil
	})
	if err != nil {
		builder.warn("workspace.scan.incomplete", "", fmt.Sprintf("could not inspect all workspace files: %v", err))
	}
}

func doctorRelativePath(root, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func (builder *doctorBuilder) ok(code, path, message string) {
	builder.add("ok", code, path, message)
}

func (builder *doctorBuilder) warn(code, path, message string) {
	builder.add("warn", code, path, message)
}

func (builder *doctorBuilder) fail(code, path, message string) {
	builder.add("fail", code, path, message)
}

func (builder *doctorBuilder) add(level, code, path, message string) {
	builder.report.Findings = append(builder.report.Findings, doctorFinding{
		Level: level, Code: code, Path: path, Message: message,
	})
}

func (builder *doctorBuilder) checkPrivateMode(code, path string, info os.FileInfo, directory bool) {
	if runtime.GOOS == "windows" {
		return
	}
	if info.Mode().Perm()&0o077 != 0 {
		builder.warn(code, path, fmt.Sprintf("permissions %04o allow group or other access; expected private %s", info.Mode().Perm(), map[bool]string{true: "directory", false: "file"}[directory]))
		return
	}
	builder.ok(code, path, "permissions are private")
}

func (builder *doctorBuilder) finish() doctorReport {
	builder.report.Summary = doctorSummary{Checks: len(builder.report.Findings)}
	for _, finding := range builder.report.Findings {
		switch finding.Level {
		case "fail":
			builder.report.Summary.Failures++
		case "warn":
			builder.report.Summary.Warnings++
		}
	}
	builder.report.OK = builder.report.Summary.Failures == 0
	return builder.report
}

func renderDoctorJSON(report doctorReport, out io.Writer) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func renderDoctorText(report doctorReport, out io.Writer) {
	fmt.Fprintln(out, "Panda doctor")
	for _, finding := range report.Findings {
		label := strings.ToUpper(finding.Level)
		if finding.Path != "" {
			fmt.Fprintf(out, "%-4s %s (%s): %s\n", label, finding.Code, finding.Path, finding.Message)
		} else {
			fmt.Fprintf(out, "%-4s %s: %s\n", label, finding.Code, finding.Message)
		}
	}
	fmt.Fprintf(out, "Checks: %d; %d failure(s), %d warning(s)\n", report.Summary.Checks, report.Summary.Failures, report.Summary.Warnings)
}
