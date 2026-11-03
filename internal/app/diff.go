package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	difflib "github.com/espinosajuanma/panda-cli/internal/diff"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func diff(ws *workspace.Workspace, args []string, out io.Writer) error {
	options, err := parseDiffOptions(args)
	if err != nil {
		return err
	}
	if options.json || options.summary {
		return renderDiffWithOptions(ws, options, out, out)
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderDiffWithOptions(ws, options, rendered, out)
	})
}

func shouldPageDiff(args []string, terminal bool, output []byte, width, height int) bool {
	if !terminal || hasDiffSummary(args) || hasDiffJSON(args) {
		return false
	}
	return !outputFitsScreen(output, width, height)
}

func hasDiffSummary(args []string) bool {
	return contains(args, "--summary") || contains(args, "-s")
}

func hasDiffJSON(args []string) bool { return contains(args, "--json") }

func outputFitsScreen(output []byte, width, height int) bool {
	return ui.OutputFitsScreen(output, width, height)
}

type diffOptions struct {
	task    string
	verbose bool
	summary bool
	json    bool
}

func parseDiffOptions(args []string) (diffOptions, error) {
	var options diffOptions
	for _, arg := range args {
		switch arg {
		case "--verbose", "-v":
			options.verbose = true
		case "--summary", "-s":
			options.summary = true
		case "--json":
			options.json = true
		default:
			if strings.HasPrefix(arg, "-") || options.task != "" {
				return diffOptions{}, errors.New("usage: panda diff [taskNumber|draft-N] [--verbose|-v] [--summary|-s] [--json]")
			}
			options.task = arg
		}
	}
	if options.json && options.summary {
		return diffOptions{}, errors.New("panda diff --json includes summary counts; do not combine --json with --summary")
	}
	return options, nil
}

func renderDiffWithOptions(ws *workspace.Workspace, options diffOptions, out, colorTarget io.Writer) error {
	report, err := collectDiffReport(ws, options)
	if err != nil {
		return err
	}
	if options.json {
		return renderDiffJSON(report, out)
	}
	return renderDiffReport(report, options, out, colorTarget)
}

func isTerminal(out io.Writer) bool                 { return ui.IsTerminal(out) }
func pageOutput(out io.Writer, output []byte) error { return ui.PageOutput(out, output) }
func configureLessPager(pager []string) []string    { return ui.ConfigureLessPager(pager) }
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
