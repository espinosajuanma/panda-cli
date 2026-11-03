package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type listOptions struct {
	statusFilter    map[string]bool
	statusSpecified bool
	allStatuses     bool
	noComplexity    bool
	noDescription   bool
	noPackage       bool
	noLabels        bool
	json            bool
}

type effortEstimate = task.Effort

type listTaskEntry struct {
	dir      string
	metadata map[string]any
}

type listGroup struct {
	status  string
	entries []listTaskEntry
	totals  effortEstimate
}

type listReport struct {
	groups       []listGroup
	total        effortEstimate
	labelNames   map[string]string
	packageNames map[string]string
}

func listTasks(ws *workspace.Workspace, args []string, out io.Writer) error {
	options, err := parseListOptions(args)
	if err != nil {
		return err
	}
	report, err := collectListReport(ws, options)
	if err != nil {
		return err
	}
	if options.json {
		return renderListJSON(report, out)
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderListReport(report, rendered, out)
	})
}

func collectListReport(ws *workspace.Workspace, options listOptions) (listReport, error) {
	labelsCatalog, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return listReport{}, err
	}
	packagesCatalog, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return listReport{}, err
	}
	report := listReport{
		groups:       make([]listGroup, 0),
		labelNames:   catalogNameIndex(labelCatalog, labelsCatalog),
		packageNames: catalogNameIndex(packageCatalog, packagesCatalog),
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return listReport{}, err
	}
	grouped := make(map[string][]listTaskEntry)
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return listReport{}, fmt.Errorf("read task metadata in %s: %w", filepath.Base(dir), err)
		}
		status := stringField(metadata, "status")
		if status == "" {
			status = "new"
		}
		if !options.allStatuses && !options.statusFilter[status] {
			continue
		}
		if options.noComplexity && !isEmptyField(metadata["complexity"]) {
			continue
		}
		if options.noDescription {
			description, err := os.ReadFile(filepath.Join(dir, "description.md"))
			if err != nil && !os.IsNotExist(err) {
				return listReport{}, fmt.Errorf("read task description in %s: %w", filepath.Base(dir), err)
			}
			if err == nil && strings.TrimSpace(string(description)) != "" {
				continue
			}
		}
		if options.noPackage && hasPackageAssignment(metadata["package"]) {
			continue
		}
		if options.noLabels && hasLabelAssignments(metadata["labels"]) {
			continue
		}
		grouped[status] = append(grouped[status], listTaskEntry{dir: dir, metadata: metadata})
	}
	statuses := orderedTaskStatuses(grouped)
	for _, status := range statuses {
		entries := grouped[status]
		sort.SliceStable(entries, func(i, j int) bool {
			return taskMetadataRankLess(entries[i].metadata, entries[j].metadata)
		})
		group := listGroup{status: status, entries: entries}
		for _, entry := range entries {
			if effort, ok := task.ComplexityEffort[strings.ToLower(strings.TrimSpace(stringField(entry.metadata, "complexity")))]; ok {
				group.totals.StoryPoints += effort.StoryPoints
				group.totals.Hours += effort.Hours
			}
		}
		report.total.StoryPoints += group.totals.StoryPoints
		report.total.Hours += group.totals.Hours
		report.groups = append(report.groups, group)
	}
	return report, nil
}

func renderListReport(report listReport, out, colorTarget io.Writer) error {
	if len(report.groups) == 0 {
		fmt.Fprintln(out, "No tasks match the filters.")
		return nil
	}
	for index, group := range report.groups {
		if index > 0 {
			fmt.Fprintln(out, colorText(colorTarget, "2", "----------------------------------------"))
		}
		metadata := make([]map[string]any, 0, len(group.entries))
		for _, entry := range group.entries {
			metadata = append(metadata, entry.metadata)
		}
		_, err := printTaskGroup(out, colorTarget, group.status, metadata, report.labelNames, report.packageNames)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "TOTAL (visible tasks): %d SP, %d hours\n", report.total.StoryPoints, report.total.Hours)
	return nil
}

func parseListOptions(args []string) (listOptions, error) {
	options := listOptions{statusFilter: make(map[string]bool)}
	for _, status := range task.DefaultStatuses {
		options.statusFilter[status] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--current":
			addQuickStatusFilter(&options, task.CurrentStatuses)
		case arg == "--staging":
			addQuickStatusFilter(&options, []string{"staging"})
		case arg == "--released":
			addQuickStatusFilter(&options, []string{"released"})
		case arg == "--discarded":
			addQuickStatusFilter(&options, []string{"discarded"})
		case arg == "--all":
			options.statusSpecified = true
			options.allStatuses = true
		case arg == "--no-complexity":
			options.noComplexity = true
		case arg == "--no-description":
			options.noDescription = true
		case arg == "--no-package":
			options.noPackage = true
		case arg == "--no-labels":
			options.noLabels = true
		case arg == "--json":
			options.json = true
		case arg == "--status":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, listUsageError()
			}
			i++
			if err := addStatusFilter(&options, args[i]); err != nil {
				return options, err
			}
		case strings.HasPrefix(arg, "--status="):
			if err := addStatusFilter(&options, strings.TrimPrefix(arg, "--status=")); err != nil {
				return options, err
			}
		default:
			return options, listUsageError()
		}
	}
	return options, nil
}

func listUsageError() error {
	return errors.New("usage: panda list [--current] [--staging] [--released] [--discarded] [--all] [--status <status,...>] [--no-complexity] [--no-description] [--no-package] [--no-labels] [--json]")
}

func addQuickStatusFilter(options *listOptions, statuses []string) {
	beginExplicitStatusFilter(options)
	for _, status := range statuses {
		options.statusFilter[status] = true
	}
}

func beginExplicitStatusFilter(options *listOptions) {
	if options.statusSpecified {
		return
	}
	options.statusSpecified = true
	options.statusFilter = make(map[string]bool)
}

func addStatusFilter(options *listOptions, value string) error {
	beginExplicitStatusFilter(options)
	known := make(map[string]bool, len(task.StatusOrder))
	for _, status := range task.StatusOrder {
		known[status] = true
	}
	parts := strings.Split(value, ",")
	added := 0
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if !known[status] {
			return fmt.Errorf("unknown status %q; valid statuses are %s", status, strings.Join(task.StatusOrder, ", "))
		}
		options.statusFilter[status] = true
		added++
	}
	if added == 0 {
		return errors.New("--status requires one or more comma-separated statuses")
	}
	options.statusSpecified = true
	return nil
}

func orderedTaskStatuses(groups map[string][]listTaskEntry) []string {
	statuses := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, status := range task.StatusOrder {
		if len(groups[status]) > 0 {
			statuses = append(statuses, status)
			seen[status] = true
		}
	}
	var remaining []string
	for status := range groups {
		if !seen[status] {
			remaining = append(remaining, status)
		}
	}
	sort.Strings(remaining)
	return append(statuses, remaining...)
}

func taskMetadataRankLess(left, right map[string]any) bool {
	leftRank, rightRank := stringField(left, "rank"), stringField(right, "rank")
	if leftRank == "" || rightRank == "" {
		if leftRank != rightRank {
			return leftRank != ""
		}
	} else if leftRank != rightRank {
		return leftRank < rightRank
	}
	return compareTaskNumbers(left["number"], right["number"]) < 0
}

func compareTaskNumbers(left, right any) int {
	leftNumber, leftErr := strconv.ParseFloat(fmt.Sprint(left), 64)
	rightNumber, rightErr := strconv.ParseFloat(fmt.Sprint(right), 64)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

func printTaskGroup(out, colorTarget io.Writer, status string, tasks []map[string]any, labelNames, packageNames map[string]string) (effortEstimate, error) {
	style := "1;32"
	if status == "released" {
		style = "1;36"
	}
	fmt.Fprintln(out, colorText(colorTarget, style, displayStatus(status)))
	var table bytes.Buffer
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tPRIORITY\tTYPE\tCOMPLEXITY\tSP\tHOURS\tPACKAGE\tLABELS\tTITLE"); err != nil {
		return effortEstimate{}, err
	}
	var total effortEstimate
	for _, currentTask := range tasks {
		number := stringField(currentTask, "number")
		if number == "" {
			number = taskLabel(currentTask)
		}
		complexity := tableCell(currentTask["complexity"])
		if complexity == "" {
			complexity = "-"
		}
		storyPoints, hours := "-", "-"
		if estimate, ok := task.ComplexityEffort[strings.ToLower(strings.TrimSpace(complexity))]; ok {
			storyPoints = strconv.Itoa(estimate.StoryPoints)
			hours = strconv.Itoa(estimate.Hours)
			total.StoryPoints += estimate.StoryPoints
			total.Hours += estimate.Hours
		}
		pkg := relationshipDisplayNameWithCatalog(currentTask["package"], packageNames)
		if pkg == "" {
			pkg = "-"
		}
		labels := labelsDisplayWithCatalog(currentTask["labels"], labelNames)
		if labels == "" {
			labels = "-"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			tableCell(number), tableCell(currentTask["priority"]), tableCell(currentTask["type"]), complexity,
			storyPoints, hours, pkg, labels, tableCell(currentTask["title"])); err != nil {
			return effortEstimate{}, err
		}
	}
	if err := writer.Flush(); err != nil {
		return effortEstimate{}, err
	}
	lines := strings.Split(strings.TrimSuffix(table.String(), "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			currentTask := tasks[i-1]
			if stringField(currentTask, "type") == "release" || status == "released" {
				line = colorText(colorTarget, "36", line)
			}
		}
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(out, "Subtotal: %d SP, %d hours\n", total.StoryPoints, total.Hours)
	return total, nil
}

func displayStatus(status string) string {
	var spaced strings.Builder
	for i, char := range status {
		if i > 0 && char >= 'A' && char <= 'Z' {
			spaced.WriteByte(' ')
		}
		spaced.WriteRune(char)
	}
	return strings.ToUpper(spaced.String())
}

func tableCell(value any) string {
	if value == nil {
		return ""
	}
	return strings.Join(strings.Fields(fmt.Sprint(value)), " ")
}

func isEmptyField(value any) bool {
	return value == nil || strings.TrimSpace(fmt.Sprint(value)) == ""
}
