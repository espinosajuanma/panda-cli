package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type taskViewOptions struct {
	task           string
	noDescription  bool
	noNotes        bool
	noReleaseNotes bool
	json           bool
}

type taskViewRow = report.TaskViewRow

type taskViewNote struct {
	Filename string `json:"filename"`
	Markdown string `json:"markdown"`
	Staged   bool   `json:"staged"`
}

type taskViewJSON struct {
	Task         map[string]any        `json:"task"`
	Description  *string               `json:"description,omitempty"`
	ReleaseNotes *taskViewReleaseNotes `json:"releaseNotes,omitempty"`
	Notes        *[]taskViewNote       `json:"notes,omitempty"`
	Context      taskViewJSONContext   `json:"context"`
}

type taskViewReleaseNotes struct {
	CustomerNotes string `json:"customerNotes"`
	InternalNotes string `json:"internalNotes"`
}

type taskViewJSONContext struct {
	Link           string                 `json:"link,omitempty"`
	SyncStatus     string                 `json:"syncStatus"`
	PendingChanges bool                   `json:"pendingChanges"`
	LocalDraft     bool                   `json:"localDraft"`
	StagedFiles    []workspace.StagedFile `json:"stagedFiles,omitempty"`
}

func viewTask(ws *workspace.Workspace, args []string, out io.Writer) error {
	return viewTaskWithAPIURL(ws, args, config.DefaultAPIURL, out)
}

func viewTaskWithAPIURL(ws *workspace.Workspace, args []string, apiURL string, out io.Writer) error {
	options, err := parseTaskViewOptions(args)
	if err != nil {
		return err
	}
	dir, metadata, err := ws.FindTask(options.task)
	if err != nil {
		return err
	}
	state, _ := ws.ReadState(stateKey(dir, metadata))
	context := buildTaskViewContextForAPIURL(ws, dir, metadata, state, apiURL)
	if options.json {
		return renderTaskViewJSON(dir, metadata, state, options, context, out)
	}

	width := 100
	if isTerminal(out) {
		if terminalWidth, _, sizeErr := ui.TerminalSize(out); sizeErr == nil && terminalWidth > 0 {
			width = terminalWidth
		}
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderTaskView(ws, dir, metadata, state, options, context, rendered, out, width)
	})
}

func parseTaskViewOptions(args []string) (taskViewOptions, error) {
	var options taskViewOptions
	for _, arg := range args {
		switch arg {
		case "--no-description":
			options.noDescription = true
		case "--no-notes":
			options.noNotes = true
		case "--no-release-notes":
			options.noReleaseNotes = true
		case "--json":
			options.json = true
		default:
			if strings.HasPrefix(arg, "-") || options.task != "" {
				return taskViewOptions{}, errors.New("usage: panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--no-release-notes] [--json]")
			}
			options.task = arg
		}
	}
	if options.task == "" {
		return taskViewOptions{}, errors.New("usage: panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--no-release-notes] [--json]")
	}
	return options, nil
}

func renderTaskView(ws *workspace.Workspace, dir string, metadata map[string]any, state workspace.State, options taskViewOptions, viewContext taskViewJSONContext, out, colorTarget io.Writer, width int) error {
	labelsCatalog, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return err
	}
	packagesCatalog, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return err
	}
	labelNames := catalogNameIndex(labelCatalog, labelsCatalog)
	packageNames := catalogNameIndex(packageCatalog, packagesCatalog)
	view := report.TaskViewText{
		Title:            taskLabel(metadata) + " — " + stringField(metadata, "title"),
		Rows:             taskViewMetadataRows(metadata, labelNames, packageNames, viewContext),
		Link:             viewContext.Link,
		ShowDescription:  !options.noDescription,
		ShowReleaseNotes: !options.noReleaseNotes,
		ShowNotes:        !options.noNotes,
	}
	if view.ShowDescription {
		description, err := os.ReadFile(filepath.Join(dir, "description.md"))
		if err != nil {
			return err
		}
		view.Description = string(description)
		view.HasDescription = true
	}
	if view.ShowReleaseNotes {
		localNotes, err := readLocalReleaseNotes(dir)
		if err != nil {
			return err
		}
		for _, field := range releaseNoteFields {
			if !localNotes.Exists[field.Name] {
				continue
			}
			view.ReleaseNotes = append(view.ReleaseNotes, report.TaskViewReleaseNote{
				Heading: releaseNoteHeading(field.Name), Markdown: localNotes.Markdown[field.Name],
			})
		}
	}
	for _, file := range taskFileReferences(metadata) {
		view.Files = append(view.Files, report.TaskViewFile{Name: file.Name, ID: file.ID, Description: file.Description})
	}
	for _, staged := range state.StagedFiles {
		name := staged.Name
		if name == "" {
			name = filepath.Base(staged.Path)
		}
		view.Files = append(view.Files, report.TaskViewFile{Name: name, Description: staged.Description, Staged: true})
	}
	if view.ShowNotes {
		notes, err := readTaskViewNotes(dir, state)
		if err != nil {
			return err
		}
		for _, note := range notes {
			view.Notes = append(view.Notes, report.TaskViewNote{Filename: note.Filename, Markdown: note.Markdown, Staged: note.Staged})
		}
	}
	return report.RenderTaskViewText(view, out, colorTarget, width)
}

func taskViewMetadataRows(metadata map[string]any, labelNames, packageNames map[string]string, viewContext taskViewJSONContext) []taskViewRow {
	rows := make([]taskViewRow, 0, 13)
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "type", label: "Type"},
		{key: "status", label: "Status"},
		{key: "priority", label: "Priority"},
		{key: "complexity", label: "Complexity"},
		{key: "package", label: "Package"},
		{key: "labels", label: "Labels"},
		{key: "assignees", label: "Assignees"},
		{key: "reviewers", label: "Reviewers"},
	} {
		value, exists := metadata[field.key]
		if !exists && (field.key == "type" || field.key == "status" || field.key == "priority") {
			continue
		}
		display := "None"
		switch field.key {
		case "package":
			display = relationshipDisplayNameWithCatalog(value, packageNames)
		case "labels":
			display = labelsDisplayWithCatalog(value, labelNames)
		case "assignees", "reviewers":
			display = assigneesDisplay(value)
		case "complexity":
			if complexity := stringField(metadata, field.key); complexity != "" {
				display = task.ComplexityLabel(complexity)
			}
		default:
			display = strings.TrimSpace(fmt.Sprint(value))
		}
		if display == "" || display == "<nil>" {
			display = "None"
		}
		rows = append(rows, report.TaskViewRow{Label: field.label, Value: display})
		if field.key == "status" && stringField(metadata, "status") == "released" {
			if release := displayRelationship(metadata["release"]); release != "" {
				rows = append(rows, report.TaskViewRow{Label: "Release", Value: release})
			}
		}
	}
	if createdBy, ok := metadata["createdBy"]; ok {
		if name := displayRelationship(createdBy); name != "" {
			rows = append(rows, report.TaskViewRow{Label: "Created by", Value: name})
		}
	}
	if createdAt, ok := metadata["createdAt"]; ok {
		if formatted := formatTaskCreatedAt(createdAt); formatted != "" {
			rows = append(rows, report.TaskViewRow{Label: "Created at", Value: formatted})
		}
	}
	rows = append(rows, report.TaskViewRow{Label: "Sync", Value: viewContext.SyncStatus})
	return rows
}

func displayRelationship(value any) string {
	if name := relationshipDisplayName(value); name != "" {
		return name
	}
	if _, ok := value.(map[string]any); ok {
		return ""
	}
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func formatTaskCreatedAt(value any) string {
	millis, ok := timestampMillis(value)
	if !ok {
		return ""
	}
	return time.UnixMilli(millis).Local().Format("2006-01-02 15:04 MST")
}

func readTaskViewNotes(dir string, state workspace.State) ([]taskViewNote, error) {
	files, err := workspace.TaskNoteFiles(dir)
	if err != nil {
		return nil, err
	}
	staged := make(map[string]bool, len(state.StagedNotes))
	for _, filename := range state.StagedNotes {
		staged[filename] = true
	}
	notes := make([]taskViewNote, 0, len(files))
	for _, file := range files {
		notes = append(notes, taskViewNote{Filename: file.Filename, Markdown: file.Markdown, Staged: staged[file.Filename]})
	}
	return notes, nil
}

func buildTaskViewContextForAPIURL(ws *workspace.Workspace, dir string, metadata map[string]any, state workspace.State, apiURL string) taskViewJSONContext {
	localDraft := isLocalDraft(dir, metadata) || state.LocalCreated
	status := "Synced"
	pending := false
	if localDraft {
		status = "Local draft"
		pending = true
	} else if state.Remote == nil || stringField(state.Remote, "id") != stringField(metadata, "id") {
		status = "Sync state unavailable"
	} else if dirty, err := taskHasLocalChanges(metadata, state, dir); err != nil {
		status = "Sync state unavailable"
	} else if dirty {
		status = "Local changes pending push"
		pending = true
	}
	link, _ := verifiedTaskLinkForAPIURL(ws, metadata, apiURL)
	return taskViewJSONContext{
		Link:           link,
		SyncStatus:     status,
		PendingChanges: pending,
		LocalDraft:     localDraft,
		StagedFiles:    state.StagedFiles,
	}
}

func renderTaskViewJSON(dir string, metadata map[string]any, state workspace.State, options taskViewOptions, viewContext taskViewJSONContext, out io.Writer) error {
	document := taskViewJSON{Task: metadata, Context: viewContext}
	if !options.noDescription {
		description, err := os.ReadFile(filepath.Join(dir, "description.md"))
		if err != nil {
			return err
		}
		markdown := string(description)
		document.Description = &markdown
	}
	if !options.noReleaseNotes {
		localNotes, err := readLocalReleaseNotes(dir)
		if err != nil {
			return err
		}
		document.ReleaseNotes = &taskViewReleaseNotes{
			CustomerNotes: localNotes.Markdown["customerNotes"],
			InternalNotes: localNotes.Markdown["internalNotes"],
		}
	}
	if !options.noNotes {
		notes, err := readTaskViewNotes(dir, state)
		if err != nil {
			return err
		}
		document.Notes = &notes
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func linkTask(ws *workspace.Workspace, args []string, out io.Writer) error {
	return linkTaskWithAPIURL(ws, args, config.DefaultAPIURL, out)
}

func linkTaskWithAPIURL(ws *workspace.Workspace, args []string, apiURL string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: panda link <taskNumber|draft-N>")
	}
	_, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	link, err := verifiedTaskLinkForAPIURL(ws, metadata, apiURL)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, link)
	return nil
}

func verifiedTaskLinkForAPIURL(ws *workspace.Workspace, metadata map[string]any, apiURL string) (string, error) {
	taskID := stringField(metadata, "id")
	if taskID == "" {
		return "", fmt.Errorf("task %s has no verified Solutions task ID; publish it with `panda push` first", taskLabel(metadata))
	}
	state, err := ws.ReadState(taskID)
	if err != nil || state.LocalCreated || state.Remote == nil || stringField(state.Remote, "id") != taskID {
		return "", fmt.Errorf("task %s has no verified remote sync baseline; run `panda pull` first", taskLabel(metadata))
	}
	if !same(valueAt(metadata, "number"), valueAt(state.Remote, "number")) {
		return "", fmt.Errorf("task %s number does not match its verified remote baseline", taskLabel(metadata))
	}
	return canonicalTaskLinkForAPIURL(apiURL, metadata["number"], taskID)
}

func canonicalTaskLink(taskNumber any, taskID string) (string, error) {
	return canonicalTaskLinkForAPIURL(config.DefaultAPIURL, taskNumber, taskID)
}

func canonicalTaskLinkForAPIURL(apiURL string, taskNumber any, taskID string) (string, error) {
	numberText := ""
	if taskNumber != nil {
		numberText = strings.TrimSpace(jsonutil.StringValue(taskNumber))
	}
	if numberText == "" {
		return "", errors.New("task number is missing")
	}
	number, err := strconv.ParseFloat(numberText, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return "", fmt.Errorf("task number %q is invalid", numberText)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", errors.New("verified task ID is missing")
	}
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Solutions API URL %q", apiURL)
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/"), "/api")
	return fmt.Sprintf("[#%s](%s/#views/%s/readOnly/%s)", numberText, strings.TrimRight(parsed.String(), "/"), url.PathEscape(solutions.TaskViewId), url.PathEscape(taskID)), nil
}
