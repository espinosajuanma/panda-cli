package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	difflib "github.com/espinosajuanma/panda-cli/internal/diff"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
)

type DiffReport struct {
	SchemaVersion int             `json:"schemaVersion"`
	Tasks         []DiffTask      `json:"tasks"`
	Catalogs      DiffCatalogs    `json:"catalogs"`
	Summary       DiffSummaryData `json:"summary"`
}

type DiffTask struct {
	Task      map[string]any `json:"task"`
	New       bool           `json:"new"`
	Discarded bool           `json:"discarded"`
	Changes   []DiffChange   `json:"changes"`
}

type DiffChange struct {
	Kind                string         `json:"kind"`
	Field               string         `json:"field,omitempty"`
	Before              *any           `json:"before,omitempty"`
	After               *any           `json:"after,omitempty"`
	Path                string         `json:"path,omitempty"`
	Filename            string         `json:"filename,omitempty"`
	Name                string         `json:"name,omitempty"`
	TextName            string         `json:"-"`
	SourcePath          string         `json:"sourcePath,omitempty"`
	Description         string         `json:"description,omitempty"`
	SourceMissing       bool           `json:"sourceMissing,omitempty"`
	Updated             bool           `json:"updated,omitempty"`
	BeforeMarkdown      *string        `json:"beforeMarkdown,omitempty"`
	AfterMarkdown       *string        `json:"afterMarkdown,omitempty"`
	Markdown            string         `json:"markdown,omitempty"`
	Lines               []DiffJSONLine `json:"lines,omitempty"`
	FinalNewlineChanged bool           `json:"finalNewlineChanged,omitempty"`
}

type DiffJSONLine struct {
	Marker string `json:"marker"`
	Text   string `json:"text"`
}

type DiffCatalogs struct {
	Labels   []DiffCatalogChange `json:"labels"`
	Packages []DiffCatalogChange `json:"packages"`
}

type DiffCatalogChange struct {
	Kind   string   `json:"kind"`
	Action string   `json:"action"`
	Name   string   `json:"name"`
	Fields []string `json:"fields,omitempty"`
}

type DiffSummaryData struct {
	Edited      int `json:"edited"`
	Discarded   int `json:"discarded"`
	New         int `json:"new"`
	Labels      int `json:"labels"`
	Packages    int `json:"packages"`
	ChangeCount int `json:"changeCount"`
}

type DiffRenderOptions struct {
	Verbose bool
	Summary bool
	Task    string
}

func RenderDiff(doc DiffReport, options DiffRenderOptions, out, colorTarget io.Writer) error {
	if options.Summary {
		fmt.Fprintf(out, "Tasks: %d edited, %d discarded, %d new\n", doc.Summary.Edited, doc.Summary.Discarded, doc.Summary.New)
		if options.Task == "" && doc.Summary.Labels+doc.Summary.Packages > 0 {
			fmt.Fprintf(out, "Project catalogs: %d label(s), %d package(s) changed\n", doc.Summary.Labels, doc.Summary.Packages)
		}
		return nil
	}
	for _, current := range doc.Tasks {
		var taskChanges strings.Builder
		stagedNotes, stagedFiles := 0, 0
		for _, change := range current.Changes {
			switch change.Kind {
			case "new-task":
				fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "1;32", "new task"))
			case "new-field":
				fmt.Fprintf(&taskChanges, "  %s: %s\n", ui.ColorText(colorTarget, "1", change.Field), ui.ColorText(colorTarget, "32", formatDiffValue(pointerValue(change.After))))
			case "field":
				fmt.Fprintf(&taskChanges, "  %s: %s %s %s\n", ui.ColorText(colorTarget, "1", change.Field), ui.ColorText(colorTarget, "31", formatDiffValue(pointerValue(change.Before))), ui.ColorText(colorTarget, "2", "->"), ui.ColorText(colorTarget, "32", formatDiffValue(pointerValue(change.After))))
			case "description":
				if options.Verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "1;36", "description.md"))
					writeLineDiff(&taskChanges, colorTarget, *change.BeforeMarkdown, *change.AfterMarkdown)
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", ui.ColorText(colorTarget, "1", "description:"), ui.ColorText(colorTarget, "32", "updated"))
				}
			case "release-note":
				if options.Verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "1;36", ReleaseNoteDisplayName(change.Field)+": "+change.Path))
					writeLineDiff(&taskChanges, colorTarget, *change.BeforeMarkdown, *change.AfterMarkdown)
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", ui.ColorText(colorTarget, "1", ReleaseNoteDisplayName(change.Field)+":"), ui.ColorText(colorTarget, "32", change.Path+" updated"))
				}
			case "edited-note":
				if options.Verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "1;36", "edited note: "+change.Path))
					writeLineDiff(&taskChanges, colorTarget, *change.BeforeMarkdown, *change.AfterMarkdown)
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", ui.ColorText(colorTarget, "1", "note:"), ui.ColorText(colorTarget, "32", change.Path+" updated"))
				}
			case "staged-note":
				stagedNotes++
				if options.Verbose {
					fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "1;36", "staged note: "+change.Path))
					writeLineDiff(&taskChanges, colorTarget, "", change.Markdown)
				}
			case "staged-file":
				stagedFiles++
				if options.Verbose {
					status := "staged file upload"
					if change.SourceMissing {
						status += " (source missing)"
					}
					fmt.Fprintf(&taskChanges, "  %s: %s\n", ui.ColorText(colorTarget, "1;36", status), change.TextName)
					fmt.Fprintf(&taskChanges, "    source: %s\n", change.SourcePath)
					if change.Description != "" {
						fmt.Fprintf(&taskChanges, "    description: %s\n", change.Description)
					}
				}
			}
		}
		if !options.Verbose && stagedNotes > 0 {
			fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "36", fmt.Sprintf("notes: %d staged", stagedNotes)))
		}
		if !options.Verbose && stagedFiles > 0 {
			fmt.Fprintf(&taskChanges, "  %s\n", ui.ColorText(colorTarget, "36", fmt.Sprintf("files: %d upload(s) staged", stagedFiles)))
		}
		if taskChanges.Len() > 0 {
			heading := fmt.Sprintf("Task %s: %s", task.Label(current.Task), jsonutil.StringField(current.Task, "title"))
			fmt.Fprintln(out, ui.ColorText(colorTarget, "1;36", heading))
			_, _ = io.WriteString(out, taskChanges.String())
			fmt.Fprintln(out)
		}
	}
	if options.Task == "" {
		catalogChanges := append(append([]DiffCatalogChange(nil), doc.Catalogs.Labels...), doc.Catalogs.Packages...)
		if len(catalogChanges) > 0 {
			lines := make([]string, 0, len(catalogChanges))
			for _, change := range catalogChanges {
				lines = append(lines, FormatCatalogDiffLine(change))
			}
			sort.Strings(lines)
			fmt.Fprintln(out, ui.ColorText(colorTarget, "1;36", "Project catalog changes"))
			for _, line := range lines {
				fmt.Fprintln(out, line)
			}
			fmt.Fprintln(out)
		}
	}
	if doc.Summary.ChangeCount == 0 {
		fmt.Fprintln(out, "No local changes.")
	}
	return nil
}

func RenderDiffJSON(doc DiffReport, out io.Writer) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func DiffLinesForJSON(before, after string) ([]DiffJSONLine, bool) {
	lines := myersDiff(splitDiffLines(before), splitDiffLines(after))
	result := make([]DiffJSONLine, 0, len(lines))
	for _, line := range lines {
		result = append(result, DiffJSONLine{Marker: string(line.marker), Text: line.text})
	}
	return result, len(lines) == 0 && before != after
}

func FormatCatalogDiffLine(change DiffCatalogChange) string {
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

func ReleaseNoteDisplayName(field string) string {
	switch field {
	case "customerNotes":
		return "customer notes"
	case "internalNotes":
		return "internal notes"
	default:
		return strings.TrimSpace(field)
	}
}

func pointerValue(value *any) any {
	if value == nil {
		return nil
	}
	return *value
}

func formatDiffValue(value any) string {
	switch value := value.(type) {
	case nil:
		return "(empty)"
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, formatDiffValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		for _, key := range []string{"label", "name", "id"} {
			if label := jsonutil.StringField(value, key); label != "" {
				return label
			}
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
	}
	return fmt.Sprint(value)
}

type diffLine struct {
	marker byte
	text   string
}

func writeLineDiff(out, colorTarget io.Writer, before, after string) {
	lines := myersDiff(splitDiffLines(before), splitDiffLines(after))
	if len(lines) == 0 && before != after {
		fmt.Fprintln(out, "    "+ui.ColorText(colorTarget, "33", "(final newline changed)"))
		return
	}
	type lineRange struct{ start, end int }
	var ranges []lineRange
	for index, line := range lines {
		if line.marker == ' ' {
			continue
		}
		start, end := max(0, index-2), min(len(lines), index+3)
		if len(ranges) > 0 && start <= ranges[len(ranges)-1].end {
			ranges[len(ranges)-1].end = end
		} else {
			ranges = append(ranges, lineRange{start: start, end: end})
		}
	}
	for index, lineRange := range ranges {
		if index > 0 {
			fmt.Fprintln(out, "    ...")
		}
		for _, line := range lines[lineRange.start:lineRange.end] {
			text := fmt.Sprintf("%c%s", line.marker, line.text)
			switch line.marker {
			case '-':
				text = ui.ColorText(colorTarget, "31", text)
			case '+':
				text = ui.ColorText(colorTarget, "32", text)
			}
			fmt.Fprintf(out, "    %s\n", text)
		}
	}
}

func splitDiffLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func myersDiff(before, after []string) []diffLine {
	lines := difflib.Myers(before, after)
	result := make([]diffLine, len(lines))
	for index, line := range lines {
		result[index] = diffLine{marker: line.Marker, text: line.Text}
	}
	return result
}
