package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type estimateOptions struct {
	statuses        []string
	statusFilter    map[string]bool
	types           []string
	typeFilter      map[string]bool
	includeExisting bool
}

var estimateTypeOrder = []string{"feature", "bug", "other", "release"}
var defaultEstimateTypes = []string{"feature", "bug", "other"}

type estimateTask struct {
	dir      string
	status   string
	metadata map[string]any
}

func estimateTasks(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	options, err := parseEstimateOptions(args)
	if err != nil {
		return err
	}
	allTasks, candidates, err := findEstimateTasks(ws, options)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		if len(allTasks) == 0 {
			fmt.Fprintf(out, "No tasks found in %s %s for types %s.\n", statusNoun(len(options.statuses)), strings.Join(options.statuses, ", "), strings.Join(options.types, ", "))
		} else {
			fmt.Fprintf(out, "No matching tasks in %s %s need a complexity (types: %s).\n", statusNoun(len(options.statuses)), strings.Join(options.statuses, ", "), strings.Join(options.types, ", "))
		}
		return nil
	}

	updated, skipped := 0, 0
	quit := false
	for index, current := range candidates {
		printEstimateTaskHeader(out, current, index+1, len(candidates))
		if isInteractiveTerminal(in, out) {
			choice, selected, err := interactiveComplexityChoice(current, in, out)
			if err != nil {
				if errors.Is(err, errSelectionCanceled) {
					quit = true
					break
				}
				return err
			}
			switch choice {
			case estimateSkip:
				skipped++
				continue
			case estimateQuit:
				quit = true
			case estimateSet:
				if err := saveTaskComplexity(current, selected, out); err != nil {
					return err
				}
				updated++
			}
		} else {
			choice, selected, err := typedComplexityChoice(current, in, out)
			if err != nil {
				return err
			}
			switch choice {
			case estimateSkip:
				skipped++
				continue
			case estimateQuit:
				quit = true
			case estimateSet:
				if err := saveTaskComplexity(current, selected, out); err != nil {
					return err
				}
				updated++
			}
		}
		if quit {
			break
		}
	}

	remaining := 0
	for _, current := range allTasks {
		if isEmptyField(current.metadata["complexity"]) {
			remaining++
		}
	}
	if quit {
		fmt.Fprint(out, "Stopped early. ")
	}
	fmt.Fprintf(out, "Estimate complete: %d updated, %d skipped, %d tasks still without complexity in %s (types: %s).\n", updated, skipped, remaining, strings.Join(options.statuses, ", "), strings.Join(options.types, ", "))
	if updated > 0 {
		fmt.Fprintln(out, "Run `panda push` to publish these local changes.")
	}
	return nil
}

func parseEstimateOptions(args []string) (estimateOptions, error) {
	options := estimateOptions{statusFilter: make(map[string]bool), typeFilter: make(map[string]bool)}
	var statusArgs []string
	var typeArgs []string
	typeSpecified := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--all" {
			options.includeExisting = true
			continue
		}
		if arg == "--type" {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return options, estimateUsageError()
			}
			index++
			typeArgs = append(typeArgs, args[index])
			typeSpecified = true
			continue
		}
		if strings.HasPrefix(arg, "--type=") {
			typeArgs = append(typeArgs, strings.TrimPrefix(arg, "--type="))
			typeSpecified = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			return options, estimateUsageError()
		}
		statusArgs = append(statusArgs, arg)
	}
	if len(statusArgs) == 0 {
		statusArgs = []string{"new"}
	}

	known := make(map[string]bool, len(task.StatusOrder))
	for _, status := range task.StatusOrder {
		known[status] = true
	}
	for _, arg := range statusArgs {
		parts := strings.Split(arg, ",")
		for _, part := range parts {
			status := strings.TrimSpace(part)
			if status == "" {
				continue
			}
			if !known[status] {
				return options, fmt.Errorf("unknown status %q; valid statuses are %s", status, strings.Join(task.StatusOrder, ", "))
			}
			options.statusFilter[status] = true
		}
	}
	if len(options.statusFilter) == 0 {
		return options, errors.New("panda estimate requires one or more statuses")
	}
	if !typeSpecified {
		typeArgs = defaultEstimateTypes
	}
	knownTypes := make(map[string]bool, len(estimateTypeOrder))
	for _, taskType := range estimateTypeOrder {
		knownTypes[taskType] = true
	}
	for _, arg := range typeArgs {
		for _, part := range strings.Split(arg, ",") {
			taskType := strings.TrimSpace(part)
			if taskType == "" {
				continue
			}
			if !knownTypes[taskType] {
				return options, fmt.Errorf("unknown task type %q; valid types are %s", taskType, strings.Join(estimateTypeOrder, ", "))
			}
			options.typeFilter[taskType] = true
		}
	}
	if len(options.typeFilter) == 0 {
		return options, errors.New("--type requires one or more comma-separated task types")
	}
	for _, taskType := range estimateTypeOrder {
		if options.typeFilter[taskType] {
			options.types = append(options.types, taskType)
		}
	}
	for _, status := range task.StatusOrder {
		if options.statusFilter[status] {
			options.statuses = append(options.statuses, status)
		}
	}
	return options, nil
}

func estimateUsageError() error {
	return errors.New("usage: panda estimate [status[,status...]] [--type <type,...>] [--all]")
}

func findEstimateTasks(ws *workspace.Workspace, options estimateOptions) ([]estimateTask, []estimateTask, error) {
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return nil, nil, err
	}
	byStatus := make(map[string][]estimateTask)
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("read task metadata in %s: %w", filepath.Base(dir), err)
		}
		status := stringField(metadata, "status")
		if status == "" {
			status = "new"
		}
		if options.statusFilter[status] && options.typeFilter[stringField(metadata, "type")] {
			byStatus[status] = append(byStatus[status], estimateTask{dir: dir, status: status, metadata: metadata})
		}
	}

	var allTasks, candidates []estimateTask
	for _, status := range options.statuses {
		group := byStatus[status]
		sort.SliceStable(group, func(i, j int) bool {
			return taskMetadataRankLess(group[i].metadata, group[j].metadata)
		})
		allTasks = append(allTasks, group...)
		for _, current := range group {
			if options.includeExisting || isEmptyField(current.metadata["complexity"]) {
				candidates = append(candidates, current)
			}
		}
	}
	return allTasks, candidates, nil
}

func printEstimateTaskHeader(out io.Writer, current estimateTask, index, total int) {
	identifier := stringField(current.metadata, "number")
	if identifier == "" {
		identifier = taskLabel(current.metadata)
	}
	title := tableCell(current.metadata["title"])
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(out, "\n%s\n", colorText(out, "1;32", fmt.Sprintf("#%s — %s", identifier, title)))
	details := []string{fmt.Sprintf("%d/%d", index, total), displayStatus(current.status)}
	if value := stringField(current.metadata, "type"); value != "" {
		details = append(details, value)
	}
	if value := stringField(current.metadata, "priority"); value != "" {
		details = append(details, value+" priority")
	}
	fmt.Fprintf(out, "  %s\n", strings.Join(details, " · "))
}

type estimateChoice int

const (
	estimateSet estimateChoice = iota
	estimateSkip
	estimateQuit
)

func interactiveComplexityChoice(current estimateTask, in io.Reader, out io.Writer) (estimateChoice, string, error) {
	for {
		action, err := readEstimateAction(in, out)
		if err != nil {
			return estimateSkip, "", err
		}
		switch action {
		case 'd', 'D':
			if err := showTaskDescription(current, out); err != nil {
				return estimateSkip, "", err
			}
		case 's', 'S':
			return estimateSkip, "", nil
		case 'q', 'Q', rune(selectorCancel):
			return estimateQuit, "", nil
		case rune(selectorEnter):
			choices := make([]selectorOption, 0, len(task.ComplexityOrder))
			for _, complexity := range task.ComplexityOrder {
				choices = append(choices, selectorOption{value: complexity, label: task.ComplexityLabel(complexity)})
			}
			selected, err := selectSingleOptionWithEscapeCancel(in, out, "Choose complexity · Esc returns to task actions", choices, stringField(current.metadata, "complexity"), true)
			if errors.Is(err, errSelectionEscaped) {
				continue
			}
			if errors.Is(err, errSelectionCanceled) {
				return estimateQuit, "", err
			}
			return estimateSet, selected, err
		}
	}
}

func typedComplexityChoice(current estimateTask, in io.Reader, out io.Writer) (estimateChoice, string, error) {
	choices := make([]string, 0, len(task.ComplexityOrder))
	for _, complexity := range task.ComplexityOrder {
		choices = append(choices, task.ComplexityLabel(complexity))
	}
	for {
		choice, err := prompt(in, out, fmt.Sprintf("Complexity [%s] (value, d=description, Enter=skip, q=quit): ", strings.Join(choices, "; ")))
		if err != nil {
			return estimateSkip, "", err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "":
			return estimateSkip, "", nil
		case "d":
			if err := showTaskDescription(current, out); err != nil {
				return estimateSkip, "", err
			}
			continue
		case "q":
			return estimateQuit, "", nil
		}
		complexity := strings.ToLower(strings.TrimSpace(choice))
		if contains(task.ComplexityOrder, complexity) {
			return estimateSet, complexity, nil
		}
		fmt.Fprintf(out, "Invalid complexity %q; choose %s.\n", choice, strings.Join(task.ComplexityOrder, ", "))
	}
}

func showTaskDescription(current estimateTask, out io.Writer) error {
	description, err := os.ReadFile(filepath.Join(current.dir, "description.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read description for task %s: %w", taskLabel(current.metadata), err)
	}
	if strings.TrimSpace(string(description)) == "" {
		description = []byte("No description.")
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		fmt.Fprintf(rendered, "%s\n\n", colorText(out, "1;32", "## Description — #"+taskLabel(current.metadata)))
		fmt.Fprintln(rendered, highlightMarkdown(out, string(description)))
		return nil
	})
}

func saveTaskComplexity(current estimateTask, complexity string, out io.Writer) error {
	updated := cloneMap(current.metadata)
	updated["complexity"] = complexity
	path := filepath.Join(current.dir, "metadata.json")
	if err := workspace.WriteMap(path, updated); err != nil {
		return fmt.Errorf("save complexity for task %s: %w", taskLabel(current.metadata), err)
	}
	current.metadata["complexity"] = complexity
	fmt.Fprintf(out, "Saved #%s: %s\n", estimateTaskNumber(current.metadata), task.ComplexityLabel(complexity))
	return nil
}

func estimateTaskNumber(metadata map[string]any) string {
	if number := stringField(metadata, "number"); number != "" {
		return number
	}
	return taskLabel(metadata)
}

func statusNoun(count int) string {
	if count == 1 {
		return "status"
	}
	return "statuses"
}
