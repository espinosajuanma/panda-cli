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
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type taskViewOptions struct {
	task          string
	noDescription bool
	noNotes       bool
	json          bool
}

type taskViewRow struct {
	label string
	value string
}

type taskViewNote struct {
	Filename string `json:"filename"`
	Markdown string `json:"markdown"`
	Staged   bool   `json:"staged"`
}

type taskViewJSON struct {
	Task        map[string]any      `json:"task"`
	Description *string             `json:"description,omitempty"`
	Notes       *[]taskViewNote     `json:"notes,omitempty"`
	Context     taskViewJSONContext `json:"context"`
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
		case "--json":
			options.json = true
		default:
			if strings.HasPrefix(arg, "-") || options.task != "" {
				return taskViewOptions{}, errors.New("usage: panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--json]")
			}
			options.task = arg
		}
	}
	if options.task == "" {
		return taskViewOptions{}, errors.New("usage: panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--json]")
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
	title := taskLabel(metadata) + " — " + stringField(metadata, "title")
	fmt.Fprintf(out, "%s\n\n", colorText(colorTarget, "1;32", "# "+title))
	fmt.Fprintln(out, colorText(colorTarget, "1;32", "## Metadata"))
	rows := taskViewMetadataRows(metadata, labelNames, packageNames, viewContext)
	writeTaskViewTable(out, colorTarget, rows, width)
	if viewContext.Link != "" {
		fmt.Fprintf(out, "%s %s\n", colorText(colorTarget, "1;36", "Task link:"), viewContext.Link)
	}
	if !options.noDescription {
		fmt.Fprintln(out, colorText(colorTarget, "1;32", "\n## Description"))
		description, err := os.ReadFile(filepath.Join(dir, "description.md"))
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(description)) == "" {
			fmt.Fprintln(out, "No description.")
		} else {
			fmt.Fprintln(out, highlightMarkdown(out, string(description)))
		}
	}
	fmt.Fprintln(out, colorText(colorTarget, "1;32", "\n## Files"))
	files := taskFileReferences(metadata)
	if len(files) == 0 {
		fmt.Fprintln(out, "No attached files.")
	} else {
		for _, file := range files {
			fmt.Fprintf(out, "- %s (id: %s)", file.Name, file.ID)
			if file.Description != "" {
				fmt.Fprintf(out, " — %s", file.Description)
			}
			fmt.Fprintln(out)
		}
	}
	for _, staged := range state.StagedFiles {
		name := staged.Name
		if name == "" {
			name = filepath.Base(staged.Path)
		}
		fmt.Fprintf(out, "- %s (staged upload)", name)
		if staged.Description != "" {
			fmt.Fprintf(out, " — %s", staged.Description)
		}
		fmt.Fprintln(out)
	}
	if !options.noNotes {
		fmt.Fprintln(out)
		fmt.Fprintln(out, colorText(colorTarget, "1;32", "## Notes"))
		notes, err := readTaskViewNotes(dir, state)
		if err != nil {
			return err
		}
		if len(notes) == 0 {
			fmt.Fprintln(out, "No notes.")
		} else {
			for _, note := range notes {
				marker := ""
				if note.Staged {
					marker = colorText(colorTarget, "1;33", " [staged]")
				}
				fmt.Fprintf(out, "%s%s\n", colorText(colorTarget, "1;36", "### "+strings.TrimSuffix(note.Filename, ".md")), marker)
				fmt.Fprintln(out, highlightMarkdown(out, note.Markdown))
			}
		}
	}
	return nil
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
		rows = append(rows, taskViewRow{label: field.label, value: display})
	}
	if createdBy, ok := metadata["createdBy"]; ok {
		if name := displayRelationship(createdBy); name != "" {
			rows = append(rows, taskViewRow{label: "Created by", value: name})
		}
	}
	if createdAt, ok := metadata["createdAt"]; ok {
		if formatted := formatTaskCreatedAt(createdAt); formatted != "" {
			rows = append(rows, taskViewRow{label: "Created at", value: formatted})
		}
	}
	rows = append(rows, taskViewRow{label: "Sync", value: viewContext.SyncStatus})
	return rows
}

func writeTaskViewTable(out, colorTarget io.Writer, rows []taskViewRow, width int) {
	labelWidth := 0
	for _, row := range rows {
		if length := utf8.RuneCountInString(row.label); length > labelWidth {
			labelWidth = length
		}
	}
	if labelWidth == 0 {
		return
	}
	valueWidth := width - labelWidth - 6
	if valueWidth < 12 {
		valueWidth = 12
	}
	indent := strings.Repeat(" ", labelWidth+4)
	for _, row := range rows {
		padding := labelWidth - utf8.RuneCountInString(row.label)
		wrapped := wrapTaskViewValue(row.value, valueWidth)
		if len(wrapped) == 0 {
			wrapped = []string{"None"}
		}
		fmt.Fprintf(out, "  %s%s  %s\n", colorText(colorTarget, "1;36", row.label), strings.Repeat(" ", padding), wrapped[0])
		for _, line := range wrapped[1:] {
			fmt.Fprintf(out, "%s%s\n", indent, line)
		}
	}
}

func wrapTaskViewValue(value string, width int) []string {
	if width < 1 {
		width = 1
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := ""
	for _, word := range words {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			runes := []rune(word)
			lines = append(lines, string(runes[:width]))
			word = string(runes[width:])
		}
		if line == "" {
			line = word
		} else if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
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
	notesDir := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	staged := make(map[string]bool, len(state.StagedNotes))
	for _, filename := range state.StagedNotes {
		staged[filename] = true
	}
	notes := make([]taskViewNote, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(notesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		notes = append(notes, taskViewNote{Filename: entry.Name(), Markdown: string(body), Staged: staged[entry.Name()]})
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
