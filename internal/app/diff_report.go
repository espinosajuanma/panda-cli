package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type diffReport struct {
	SchemaVersion int             `json:"schemaVersion"`
	Tasks         []diffTask      `json:"tasks"`
	Catalogs      diffCatalogs    `json:"catalogs"`
	Summary       diffSummaryData `json:"summary"`
}

type diffTask struct {
	Task      map[string]any `json:"task"`
	New       bool           `json:"new"`
	Discarded bool           `json:"discarded"`
	Changes   []diffChange   `json:"changes"`
}

type diffChange struct {
	Kind                string         `json:"kind"`
	Field               string         `json:"field,omitempty"`
	Before              *any           `json:"before,omitempty"`
	After               *any           `json:"after,omitempty"`
	Path                string         `json:"path,omitempty"`
	Filename            string         `json:"filename,omitempty"`
	Name                string         `json:"name,omitempty"`
	textName            string         `json:"-"`
	SourcePath          string         `json:"sourcePath,omitempty"`
	Description         string         `json:"description,omitempty"`
	SourceMissing       bool           `json:"sourceMissing,omitempty"`
	Updated             bool           `json:"updated,omitempty"`
	BeforeMarkdown      *string        `json:"beforeMarkdown,omitempty"`
	AfterMarkdown       *string        `json:"afterMarkdown,omitempty"`
	Markdown            string         `json:"markdown,omitempty"`
	Lines               []diffJSONLine `json:"lines,omitempty"`
	FinalNewlineChanged bool           `json:"finalNewlineChanged,omitempty"`
}

type diffJSONLine struct {
	Marker string `json:"marker"`
	Text   string `json:"text"`
}

type diffCatalogs struct {
	Labels   []diffCatalogChange `json:"labels"`
	Packages []diffCatalogChange `json:"packages"`
}

type diffCatalogChange struct {
	Kind   string   `json:"kind"`
	Action string   `json:"action"`
	Name   string   `json:"name"`
	Fields []string `json:"fields,omitempty"`
}

type diffSummaryData struct {
	Edited      int `json:"edited"`
	Discarded   int `json:"discarded"`
	New         int `json:"new"`
	Labels      int `json:"labels"`
	Packages    int `json:"packages"`
	ChangeCount int `json:"changeCount"`
}

func collectDiffReport(ws *workspace.Workspace, options diffOptions) (diffReport, error) {
	report := diffReport{
		SchemaVersion: 1,
		Tasks:         make([]diffTask, 0),
		Catalogs: diffCatalogs{
			Labels:   make([]diffCatalogChange, 0),
			Packages: make([]diffCatalogChange, 0),
		},
	}
	selectedDir := ""
	var err error
	if options.task != "" {
		selectedDir, _, err = ws.FindTask(options.task)
		if err != nil {
			return diffReport{}, err
		}
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return diffReport{}, err
	}
	for _, dir := range dirs {
		local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		if selectedDir != "" && dir != selectedDir {
			continue
		}
		key := stateKey(dir, local)
		state, stateErr := ws.ReadState(key)
		newTask := stateErr != nil || state.LocalCreated
		discardedTask := !newTask && stringField(local, "status") == "discarded" && stringField(state.Remote, "status") != "discarded"
		taskChanges := diffTask{Task: local, New: newTask, Discarded: discardedTask, Changes: make([]diffChange, 0)}
		taskChanged := false
		if newTask {
			taskChanges.Changes = append(taskChanges.Changes, diffChange{Kind: "new-task"})
			for _, field := range []string{"type", "priority", "complexity"} {
				value := valueAt(local, field)
				taskChanges.Changes = append(taskChanges.Changes, diffChange{Kind: "new-field", Field: field, After: anyPointer(value)})
			}
			report.Summary.ChangeCount++
		} else {
			for _, field := range editableFields {
				localValue, remoteValue := valueAt(local, field), valueAt(state.Remote, field)
				if !same(localValue, remoteValue) {
					taskChanges.Changes = append(taskChanges.Changes, diffChange{
						Kind: "field", Field: field, Before: anyPointer(remoteValue), After: anyPointer(localValue),
					})
					report.Summary.ChangeCount++
					taskChanged = true
				}
			}
		}
		baseMarkdown := ""
		if stateErr == nil && !state.LocalCreated {
			baseMarkdown, err = content.HTMLToMarkdown(stringField(state.Remote, "description"))
			if err != nil {
				return diffReport{}, err
			}
		}
		localMarkdown, readErr := os.ReadFile(filepath.Join(dir, "description.md"))
		if readErr == nil && string(localMarkdown) != baseMarkdown {
			change := diffChange{Kind: "description", Path: "description.md", Updated: true}
			if options.verbose {
				before, after := baseMarkdown, string(localMarkdown)
				change.BeforeMarkdown, change.AfterMarkdown = &before, &after
				change.Lines, change.FinalNewlineChanged = diffLinesForJSON(before, after)
			}
			taskChanges.Changes = append(taskChanges.Changes, change)
			report.Summary.ChangeCount++
			taskChanged = true
		}
		if stateErr == nil && !state.LocalCreated {
			noteChanges, noteErr := localNoteChanges(dir, state.Remote)
			if noteErr != nil {
				return diffReport{}, fmt.Errorf("inspect notes for %s: %w", taskLabel(local), noteErr)
			}
			filenames := sortedMapKeys(noteChanges)
			var baseNotes map[string]string
			if options.verbose && len(filenames) > 0 {
				baseNotes, err = baselineNoteMarkdown(state.Remote)
				if err != nil {
					return diffReport{}, err
				}
			}
			for _, filename := range filenames {
				change := diffChange{Kind: "edited-note", Filename: filename, Path: filepath.ToSlash(filepath.Join("notes", filename)), Updated: true}
				if options.verbose {
					before, after := baseNotes[filename], noteChanges[filename]
					change.BeforeMarkdown, change.AfterMarkdown = &before, &after
					change.Lines, change.FinalNewlineChanged = diffLinesForJSON(before, after)
				}
				taskChanges.Changes = append(taskChanges.Changes, change)
				report.Summary.ChangeCount++
				taskChanged = true
			}
		}
		if stateErr == nil {
			for _, filename := range state.StagedNotes {
				change := diffChange{Kind: "staged-note", Filename: filename, Path: filepath.ToSlash(filepath.Join("notes", filename))}
				if options.verbose {
					note, noteErr := os.ReadFile(filepath.Join(dir, "notes", filename))
					if noteErr != nil {
						return diffReport{}, fmt.Errorf("read staged note %q: %w", filename, noteErr)
					}
					change.Markdown = string(note)
					change.Lines, change.FinalNewlineChanged = diffLinesForJSON("", string(note))
				}
				taskChanges.Changes = append(taskChanges.Changes, change)
				report.Summary.ChangeCount++
				taskChanged = true
			}
			for _, staged := range state.StagedFiles {
				name := staged.Name
				if name == "" {
					name = filepath.Base(staged.Path)
				}
				_, statErr := os.Stat(staged.Path)
				taskChanges.Changes = append(taskChanges.Changes, diffChange{
					Kind: "staged-file", Name: name, textName: staged.Name, SourcePath: staged.Path,
					Description: staged.Description, SourceMissing: statErr != nil,
				})
				report.Summary.ChangeCount++
				taskChanged = true
			}
		}
		if newTask {
			report.Summary.New++
		} else if taskChanged {
			if discardedTask {
				report.Summary.Discarded++
			} else {
				report.Summary.Edited++
			}
		}
		if len(taskChanges.Changes) > 0 {
			report.Tasks = append(report.Tasks, taskChanges)
		}
	}
	catalogs, labelChanges, packageChanges, err := projectCatalogDiff(ws)
	if err != nil {
		return diffReport{}, fmt.Errorf("inspect project catalogs: %w", err)
	}
	if options.task == "" {
		for _, change := range catalogs {
			if change.Kind == string(labelCatalog) {
				report.Catalogs.Labels = append(report.Catalogs.Labels, change)
			} else {
				report.Catalogs.Packages = append(report.Catalogs.Packages, change)
			}
		}
		report.Summary.Labels = labelChanges
		report.Summary.Packages = packageChanges
		report.Summary.ChangeCount += labelChanges + packageChanges
	}
	return report, nil
}

func renderDiffReport(report diffReport, options diffOptions, out, colorTarget io.Writer) error {
	if options.summary {
		fmt.Fprintf(out, "Tasks: %d edited, %d discarded, %d new\n", report.Summary.Edited, report.Summary.Discarded, report.Summary.New)
		if options.task == "" && report.Summary.Labels+report.Summary.Packages > 0 {
			fmt.Fprintf(out, "Project catalogs: %d label(s), %d package(s) changed\n", report.Summary.Labels, report.Summary.Packages)
		}
		return nil
	}
	for _, current := range report.Tasks {
		var taskChanges strings.Builder
		stagedNotes, stagedFiles := 0, 0
		for _, change := range current.Changes {
			switch change.Kind {
			case "new-task":
				fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;32", "new task"))
			case "new-field":
				fmt.Fprintf(&taskChanges, "  %s: %s\n", colorText(colorTarget, "1", change.Field), colorText(colorTarget, "32", formatDiffValue(pointerValue(change.After))))
			case "field":
				fmt.Fprintf(&taskChanges, "  %s: %s %s %s\n", colorText(colorTarget, "1", change.Field), colorText(colorTarget, "31", formatDiffValue(pointerValue(change.Before))), colorText(colorTarget, "2", "->"), colorText(colorTarget, "32", formatDiffValue(pointerValue(change.After))))
			case "description":
				if options.verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "description.md"))
					writeLineDiff(&taskChanges, colorTarget, *change.BeforeMarkdown, *change.AfterMarkdown)
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", colorText(colorTarget, "1", "description:"), colorText(colorTarget, "32", "updated"))
				}
			case "edited-note":
				if options.verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "edited note: "+change.Path))
					writeLineDiff(&taskChanges, colorTarget, *change.BeforeMarkdown, *change.AfterMarkdown)
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", colorText(colorTarget, "1", "note:"), colorText(colorTarget, "32", change.Path+" updated"))
				}
			case "staged-note":
				stagedNotes++
				if options.verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "staged note: "+change.Path))
					writeLineDiff(&taskChanges, colorTarget, "", change.Markdown)
				}
			case "staged-file":
				stagedFiles++
				if options.verbose {
					status := "staged file upload"
					if change.SourceMissing {
						status += " (source missing)"
					}
					fmt.Fprintf(&taskChanges, "  %s: %s\n", colorText(colorTarget, "1;36", status), change.textName)
					fmt.Fprintf(&taskChanges, "    source: %s\n", change.SourcePath)
					if change.Description != "" {
						fmt.Fprintf(&taskChanges, "    description: %s\n", change.Description)
					}
				}
			}
		}
		if !options.verbose && stagedNotes > 0 {
			fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "36", fmt.Sprintf("notes: %d staged", stagedNotes)))
		}
		if !options.verbose && stagedFiles > 0 {
			fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "36", fmt.Sprintf("files: %d upload(s) staged", stagedFiles)))
		}
		if taskChanges.Len() > 0 {
			heading := fmt.Sprintf("Task %s: %s", taskLabel(current.Task), stringField(current.Task, "title"))
			fmt.Fprintln(out, colorText(colorTarget, "1;36", heading))
			_, _ = io.WriteString(out, taskChanges.String())
			fmt.Fprintln(out)
		}
	}
	if options.task == "" {
		catalogChanges := append(append([]diffCatalogChange(nil), report.Catalogs.Labels...), report.Catalogs.Packages...)
		if len(catalogChanges) > 0 {
			lines := make([]string, 0, len(catalogChanges))
			for _, change := range catalogChanges {
				lines = append(lines, formatCatalogDiffLine(change))
			}
			sort.Strings(lines)
			fmt.Fprintln(out, colorText(colorTarget, "1;36", "Project catalog changes"))
			for _, line := range lines {
				fmt.Fprintln(out, line)
			}
			fmt.Fprintln(out)
		}
	}
	if report.Summary.ChangeCount == 0 {
		fmt.Fprintln(out, "No local changes.")
	}
	return nil
}

func renderDiffJSON(report diffReport, out io.Writer) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func baselineNoteMarkdown(remote map[string]any) (map[string]string, error) {
	notes, err := getRemoteNotes(remote)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(notes))
	for _, note := range notes {
		result[note.filename] = note.markdown
	}
	return result, nil
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func anyPointer(value any) *any { return &value }

func pointerValue(value *any) any {
	if value == nil {
		return nil
	}
	return *value
}

func diffLinesForJSON(before, after string) ([]diffJSONLine, bool) {
	lines := myersDiff(splitDiffLines(before), splitDiffLines(after))
	result := make([]diffJSONLine, 0, len(lines))
	for _, line := range lines {
		result = append(result, diffJSONLine{Marker: string(line.marker), Text: line.text})
	}
	return result, len(lines) == 0 && before != after
}

func formatCatalogDiffLine(change diffCatalogChange) string {
	switch change.Action {
	case "new":
		return fmt.Sprintf("  new %s: %s", strings.TrimSuffix(change.Kind, "s"), change.Name)
	case "edited":
		return fmt.Sprintf("  edited %s: %s (%s)", strings.TrimSuffix(change.Kind, "s"), change.Name, strings.Join(change.Fields, ", "))
	case "deleted":
		return fmt.Sprintf("  deleted %s: %s", strings.TrimSuffix(change.Kind, "s"), change.Name)
	default:
		return ""
	}
}
