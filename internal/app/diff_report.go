package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type diffReport = report.DiffReport

type diffTask = report.DiffTask

type diffChange = report.DiffChange

type diffJSONLine = report.DiffJSONLine

type diffCatalogs = report.DiffCatalogs

type diffCatalogChange = report.DiffCatalogChange

type diffSummaryData = report.DiffSummaryData

func anyPointer(value any) *any { return &value }

func pointerValue(value *any) any {
	if value == nil {
		return nil
	}
	return *value
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func collectDiffReport(ws *workspace.Workspace, options diffOptions) (diffReport, error) {
	doc := report.DiffReport{
		SchemaVersion: 1,
		Tasks:         make([]diffTask, 0),
		Catalogs: report.DiffCatalogs{
			Labels:   make([]report.DiffCatalogChange, 0),
			Packages: make([]report.DiffCatalogChange, 0),
		},
	}
	selectedDir := ""
	var err error
	if options.task != "" {
		selectedDir, _, err = ws.FindTask(options.task)
		if err != nil {
			return report.DiffReport{}, err
		}
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return report.DiffReport{}, err
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
		taskChanges := report.DiffTask{Task: local, New: newTask, Discarded: discardedTask, Changes: make([]diffChange, 0)}
		taskChanged := false
		if newTask {
			taskChanges.Changes = append(taskChanges.Changes, report.DiffChange{Kind: "new-task"})
			for _, field := range []string{"type", "priority", "complexity"} {
				value := valueAt(local, field)
				taskChanges.Changes = append(taskChanges.Changes, report.DiffChange{Kind: "new-field", Field: field, After: anyPointer(value)})
			}
			doc.Summary.ChangeCount++
		} else {
			for _, field := range editableFields {
				localValue, remoteValue := valueAt(local, field), valueAt(state.Remote, field)
				if !same(localValue, remoteValue) {
					taskChanges.Changes = append(taskChanges.Changes, report.DiffChange{
						Kind: "field", Field: field, Before: anyPointer(remoteValue), After: anyPointer(localValue),
					})
					doc.Summary.ChangeCount++
					taskChanged = true
				}
			}
		}
		baseMarkdown := ""
		if stateErr == nil && !state.LocalCreated {
			baseMarkdown, err = content.HTMLToMarkdown(stringField(state.Remote, "description"))
			if err != nil {
				return report.DiffReport{}, err
			}
		}
		localMarkdown, readErr := os.ReadFile(filepath.Join(dir, "description.md"))
		if readErr == nil && string(localMarkdown) != baseMarkdown {
			change := report.DiffChange{Kind: "description", Path: "description.md", Updated: true}
			if options.verbose {
				before, after := baseMarkdown, string(localMarkdown)
				change.BeforeMarkdown, change.AfterMarkdown = &before, &after
				change.Lines, change.FinalNewlineChanged = report.DiffLinesForJSON(before, after)
			}
			taskChanges.Changes = append(taskChanges.Changes, change)
			doc.Summary.ChangeCount++
			taskChanged = true
		}
		if stateErr == nil && !state.LocalCreated {
			releaseChanges, releaseErr := releaseNoteChanges(dir, state.Remote)
			if releaseErr != nil {
				return report.DiffReport{}, fmt.Errorf("inspect release notes for %s: %w", taskLabel(local), releaseErr)
			}
			for _, field := range releaseNoteFields {
				markdown, changed := releaseChanges[field.Name]
				if !changed {
					continue
				}
				change := report.DiffChange{Kind: "release-note", Field: field.Name, Path: field.Filename, Updated: true}
				if options.verbose {
					before, baselineErr := releaseNoteMarkdown(state.Remote, field.Name)
					if baselineErr != nil {
						return report.DiffReport{}, baselineErr
					}
					after := markdown
					change.BeforeMarkdown, change.AfterMarkdown = &before, &after
					change.Lines, change.FinalNewlineChanged = report.DiffLinesForJSON(before, after)
				}
				taskChanges.Changes = append(taskChanges.Changes, change)
				doc.Summary.ChangeCount++
				taskChanged = true
			}
		}
		if stateErr == nil && !state.LocalCreated {
			noteChanges, noteErr := localNoteChanges(dir, state.Remote)
			if noteErr != nil {
				return report.DiffReport{}, fmt.Errorf("inspect notes for %s: %w", taskLabel(local), noteErr)
			}
			filenames := sortedMapKeys(noteChanges)
			var baseNotes map[string]string
			if options.verbose && len(filenames) > 0 {
				baseNotes, err = baselineNoteMarkdown(state.Remote)
				if err != nil {
					return report.DiffReport{}, err
				}
			}
			for _, filename := range filenames {
				change := report.DiffChange{Kind: "edited-note", Filename: filename, Path: filepath.ToSlash(filepath.Join("notes", filename)), Updated: true}
				if options.verbose {
					before, after := baseNotes[filename], noteChanges[filename]
					change.BeforeMarkdown, change.AfterMarkdown = &before, &after
					change.Lines, change.FinalNewlineChanged = report.DiffLinesForJSON(before, after)
				}
				taskChanges.Changes = append(taskChanges.Changes, change)
				doc.Summary.ChangeCount++
				taskChanged = true
			}
		}
		if stateErr == nil {
			for _, filename := range state.StagedNotes {
				change := report.DiffChange{Kind: "staged-note", Filename: filename, Path: filepath.ToSlash(filepath.Join("notes", filename))}
				if options.verbose {
					note, noteErr := os.ReadFile(filepath.Join(dir, "notes", filename))
					if noteErr != nil {
						return report.DiffReport{}, fmt.Errorf("read staged note %q: %w", filename, noteErr)
					}
					change.Markdown = string(note)
					change.Lines, change.FinalNewlineChanged = report.DiffLinesForJSON("", string(note))
				}
				taskChanges.Changes = append(taskChanges.Changes, change)
				doc.Summary.ChangeCount++
				taskChanged = true
			}
			for _, staged := range state.StagedFiles {
				name := staged.Name
				if name == "" {
					name = filepath.Base(staged.Path)
				}
				_, statErr := os.Stat(staged.Path)
				taskChanges.Changes = append(taskChanges.Changes, report.DiffChange{
					Kind: "staged-file", Name: name, TextName: staged.Name, SourcePath: staged.Path,
					Description: staged.Description, SourceMissing: statErr != nil,
				})
				doc.Summary.ChangeCount++
				taskChanged = true
			}
		}
		if newTask {
			doc.Summary.New++
		} else if taskChanged {
			if discardedTask {
				doc.Summary.Discarded++
			} else {
				doc.Summary.Edited++
			}
		}
		if len(taskChanges.Changes) > 0 {
			doc.Tasks = append(doc.Tasks, taskChanges)
		}
	}
	catalogs, labelChanges, packageChanges, err := projectCatalogDiff(ws)
	if err != nil {
		return report.DiffReport{}, fmt.Errorf("inspect project catalogs: %w", err)
	}
	if options.task == "" {
		for _, change := range catalogs {
			if change.Kind == string(labelCatalog) {
				doc.Catalogs.Labels = append(doc.Catalogs.Labels, change)
			} else {
				doc.Catalogs.Packages = append(doc.Catalogs.Packages, change)
			}
		}
		doc.Summary.Labels = labelChanges
		doc.Summary.Packages = packageChanges
		doc.Summary.ChangeCount += labelChanges + packageChanges
	}
	return doc, nil
}
