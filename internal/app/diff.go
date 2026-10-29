package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/content"
	difflib "github.com/espinosajuanma/panda-cli/internal/diff"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func diff(ws *workspace.Workspace, args []string, out io.Writer) error {
	if hasDiffSummary(args) {
		return renderDiff(ws, args, out, out)
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderDiff(ws, args, rendered, out)
	})
}

func shouldPageDiff(args []string, terminal bool, output []byte, width, height int) bool {
	if !terminal || hasDiffSummary(args) {
		return false
	}
	return !outputFitsScreen(output, width, height)
}

func hasDiffSummary(args []string) bool {
	return contains(args, "--summary") || contains(args, "-s")
}

func outputFitsScreen(output []byte, width, height int) bool {
	return ui.OutputFitsScreen(output, width, height)
}

func renderDiff(ws *workspace.Workspace, args []string, out, colorTarget io.Writer) error {
	options, err := parseDiffOptions(args)
	if err != nil {
		return err
	}
	selectedDir := ""
	if options.task != "" {
		selectedDir, _, err = ws.FindTask(options.task)
		if err != nil {
			return err
		}
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	changes := 0
	counts := diffSummary{}
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
		taskChanged := false
		var taskChanges bytes.Buffer
		if newTask {
			fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;32", "new task"))
			for _, field := range []string{"type", "priority", "complexity"} {
				fmt.Fprintf(&taskChanges, "  %s: %s\n", colorText(colorTarget, "1", field), colorText(colorTarget, "32", formatDiffValue(valueAt(local, field))))
			}
			changes++
		} else {
			for _, field := range editableFields {
				if !same(valueAt(local, field), valueAt(state.Remote, field)) {
					fmt.Fprintf(&taskChanges, "  %s: %s %s %s\n", colorText(colorTarget, "1", field), colorText(colorTarget, "31", formatDiffValue(valueAt(state.Remote, field))), colorText(colorTarget, "2", "->"), colorText(colorTarget, "32", formatDiffValue(valueAt(local, field))))
					changes++
					taskChanged = true
				}
			}
		}
		baseMarkdown := ""
		if stateErr == nil && !state.LocalCreated {
			baseMarkdown, err = content.HTMLToMarkdown(stringField(state.Remote, "description"))
			if err != nil {
				return err
			}
		}
		localMarkdown, readErr := os.ReadFile(filepath.Join(dir, "description.md"))
		if readErr == nil && string(localMarkdown) != baseMarkdown {
			if options.verbose {
				fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "description.md"))
				writeLineDiff(&taskChanges, colorTarget, baseMarkdown, string(localMarkdown))
			} else {
				fmt.Fprintf(&taskChanges, "  %s %s\n", colorText(colorTarget, "1", "description:"), colorText(colorTarget, "32", "updated"))
			}
			changes++
			taskChanged = true
		}
		if stateErr == nil && !state.LocalCreated {
			noteChanges, noteErr := localNoteChanges(dir, state.Remote)
			if noteErr != nil {
				return fmt.Errorf("inspect notes for %s: %w", taskLabel(local), noteErr)
			}
			filenames := make([]string, 0, len(noteChanges))
			for filename := range noteChanges {
				filenames = append(filenames, filename)
			}
			sort.Strings(filenames)
			for _, filename := range filenames {
				if options.verbose {
					baseNotes, noteErr := getRemoteNotes(state.Remote)
					if noteErr != nil {
						return noteErr
					}
					before := ""
					for _, note := range baseNotes {
						if note.filename == filename {
							before = note.markdown
							break
						}
					}
					fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "edited note: "+filepath.ToSlash(filepath.Join("notes", filename))))
					writeLineDiff(&taskChanges, colorTarget, before, noteChanges[filename])
				} else {
					fmt.Fprintf(&taskChanges, "  %s %s\n", colorText(colorTarget, "1", "note:"), colorText(colorTarget, "32", filepath.ToSlash(filepath.Join("notes", filename))+" updated"))
				}
				changes++
				taskChanged = true
			}
		}
		if stateErr == nil && len(state.StagedNotes) > 0 {
			if options.verbose {
				for _, filename := range state.StagedNotes {
					note, noteErr := os.ReadFile(filepath.Join(dir, "notes", filename))
					if noteErr != nil {
						return fmt.Errorf("read staged note %q: %w", filename, noteErr)
					}
					notePath := filepath.ToSlash(filepath.Join("notes", filename))
					fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "1;36", "staged note: "+notePath))
					writeLineDiff(&taskChanges, colorTarget, "", string(note))
				}
			} else {
				fmt.Fprintf(&taskChanges, "  %s\n", colorText(colorTarget, "36", fmt.Sprintf("notes: %d staged", len(state.StagedNotes))))
			}
			changes += len(state.StagedNotes)
			taskChanged = true
		}
		if newTask {
			counts.new++
		} else if taskChanged {
			if discardedTask {
				counts.discarded++
			} else {
				counts.edited++
			}
		}
		if !options.summary && taskChanges.Len() > 0 {
			heading := fmt.Sprintf("Task %s: %s", taskLabel(local), stringField(local, "title"))
			fmt.Fprintln(out, colorText(colorTarget, "1;36", heading))
			_, _ = taskChanges.WriteTo(out)
			fmt.Fprintln(out)
		}
	}
	catalogLines, labelChanges, packageChanges, err := projectCatalogDiff(ws)
	if err != nil {
		return fmt.Errorf("inspect project catalogs: %w", err)
	}
	catalogChanges := labelChanges + packageChanges
	if options.task == "" {
		changes += catalogChanges
	}
	if options.summary {
		fmt.Fprintf(out, "Tasks: %d edited, %d discarded, %d new\n", counts.edited, counts.discarded, counts.new)
		if options.task == "" && catalogChanges > 0 {
			fmt.Fprintf(out, "Project catalogs: %d label(s), %d package(s) changed\n", labelChanges, packageChanges)
		}
		return nil
	}
	if options.task == "" && len(catalogLines) > 0 {
		fmt.Fprintln(out, colorText(colorTarget, "1;36", "Project catalog changes"))
		for _, line := range catalogLines {
			fmt.Fprintln(out, line)
		}
		fmt.Fprintln(out)
	}
	if changes == 0 {
		fmt.Fprintln(out, "No local changes.")
	}
	return nil
}

type diffSummary struct{ edited, discarded, new int }
type diffOptions struct {
	task    string
	verbose bool
	summary bool
}

func parseDiffOptions(args []string) (diffOptions, error) {
	var options diffOptions
	for _, arg := range args {
		switch arg {
		case "--verbose", "-v":
			options.verbose = true
		case "--summary", "-s":
			options.summary = true
		default:
			if strings.HasPrefix(arg, "-") || options.task != "" {
				return diffOptions{}, errors.New("usage: panda diff [taskNumber|draft-N] [--verbose|-v] [--summary|-s]")
			}
			options.task = arg
		}
	}
	return options, nil
}

func isTerminal(out io.Writer) bool                 { return ui.IsTerminal(out) }
func pageOutput(out io.Writer, output []byte) error { return ui.PageOutput(out, output) }
func configureLessPager(pager []string) []string    { return ui.ConfigureLessPager(pager) }
func hasLessOption(args []string, shortOptions, longOption string) bool {
	return ui.HasLessOption(args, shortOptions, longOption)
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
			if label := stringField(value, key); label != "" {
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
		fmt.Fprintln(out, "    "+colorText(colorTarget, "33", "(final newline changed)"))
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
				text = colorText(colorTarget, "31", text)
			case '+':
				text = colorText(colorTarget, "32", text)
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
