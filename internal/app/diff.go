package app

import (
	"errors"
	"io"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
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
	doc, err := collectDiffReport(ws, options)
	if err != nil {
		return err
	}
	if options.json {
		return report.RenderDiffJSON(doc, out)
	}
	return report.RenderDiff(doc, report.DiffRenderOptions{Verbose: options.verbose, Summary: options.summary, Task: options.task}, out, colorTarget)
}

func isTerminal(out io.Writer) bool                 { return ui.IsTerminal(out) }
func pageOutput(out io.Writer, output []byte) error { return ui.PageOutput(out, output) }
func configureLessPager(pager []string) []string    { return ui.ConfigureLessPager(pager) }
