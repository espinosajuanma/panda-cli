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
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
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
	assignees       []string
	reviewers       []string
	json            bool
	wide            bool
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
	groups        []listGroup
	total         effortEstimate
	labelNames    map[string]string
	packageNames  map[string]string
	showAssignees bool
	showReviewers bool
}

func listTasks(ws *workspace.Workspace, args []string, out io.Writer) error {
	return listTasksForUser(ws, args, "", out)
}

func listTasksForUser(ws *workspace.Workspace, args []string, userEmail string, out io.Writer) error {
	options, err := parseListOptions(args)
	if err != nil {
		return err
	}
	if options.json && options.wide {
		return errors.New("--wide cannot be combined with --json")
	}
	report, err := collectListReportForUser(ws, options, userEmail)
	if err != nil {
		return err
	}
	if options.json {
		return renderListJSON(report, out)
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderListReport(report, rendered, out, options.wide)
	})
}

func collectListReportForUser(ws *workspace.Workspace, options listOptions, userEmail string) (listReport, error) {
	var assigneeFilter, reviewerFilter relationFilter
	if len(options.assignees) > 0 || len(options.reviewers) > 0 {
		var members []map[string]any
		available := false
		if relationSelectorsNeedMembers(options.assignees) || relationSelectorsNeedMembers(options.reviewers) {
			var err error
			members, available, err = projectMembersIfAvailable(ws)
			if err != nil {
				return listReport{}, err
			}
		}
		var err error
		assigneeFilter, err = resolveRelationFilter(members, available, options.assignees, userEmail, "assignee")
		if err != nil {
			return listReport{}, err
		}
		reviewerFilter, err = resolveRelationFilter(members, available, options.reviewers, userEmail, "reviewer")
		if err != nil {
			return listReport{}, err
		}
	}
	labelsCatalog, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return listReport{}, err
	}
	packagesCatalog, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return listReport{}, err
	}
	report := listReport{
		groups:        make([]listGroup, 0),
		labelNames:    catalogNameIndex(labelCatalog, labelsCatalog),
		packageNames:  catalogNameIndex(packageCatalog, packagesCatalog),
		showAssignees: len(options.assignees) > 0,
		showReviewers: len(options.reviewers) > 0,
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
		if !assigneeFilter.matches(metadata["assignees"]) || !reviewerFilter.matches(metadata["reviewers"]) {
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

func renderListReport(report listReport, out, colorTarget io.Writer, wide bool) error {
	if len(report.groups) == 0 {
		fmt.Fprintln(out, "No tasks match the filters.")
		return nil
	}
	terminal := isTerminal(colorTarget)
	terminalWidth := 0
	if terminal && !wide {
		terminalWidth, _, _ = ui.TerminalSize(colorTarget)
	}
	width := listTableWidth(wide, terminal, terminalWidth)
	for index, group := range report.groups {
		if index > 0 {
			fmt.Fprintln(out, colorText(colorTarget, "2", "----------------------------------------"))
		}
		metadata := make([]map[string]any, 0, len(group.entries))
		for _, entry := range group.entries {
			metadata = append(metadata, entry.metadata)
		}
		_, err := printTaskGroup(out, colorTarget, group.status, metadata, report.labelNames, report.packageNames, report.showAssignees, report.showReviewers, width)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "TOTAL (visible tasks): %d SP, %d hours\n", report.total.StoryPoints, report.total.Hours)
	return nil
}

func listTableWidth(wide, terminal bool, terminalWidth int) int {
	if wide || !terminal {
		return 0
	}
	if terminalWidth < 1 {
		return 80
	}
	return terminalWidth
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
		case arg == "--assignee" || arg == "--reviewer":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, listUsageError()
			}
			i++
			if arg == "--assignee" {
				if err := appendRelationSelectors(&options.assignees, args[i], "--assignee"); err != nil {
					return options, err
				}
			} else if err := appendRelationSelectors(&options.reviewers, args[i], "--reviewer"); err != nil {
				return options, err
			}
		case arg == "--json":
			options.json = true
		case arg == "--wide":
			options.wide = true
		case strings.HasPrefix(arg, "--assignee="):
			if err := appendRelationSelectors(&options.assignees, strings.TrimPrefix(arg, "--assignee="), "--assignee"); err != nil {
				return options, err
			}
		case strings.HasPrefix(arg, "--reviewer="):
			if err := appendRelationSelectors(&options.reviewers, strings.TrimPrefix(arg, "--reviewer="), "--reviewer"); err != nil {
				return options, err
			}
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
	return errors.New("usage: panda list [--current] [--staging] [--released] [--discarded] [--all] [--status <status,...>] [--no-complexity] [--no-description] [--no-package] [--no-labels] [--assignee <member,...>] [--reviewer <member,...>] [--wide] [--json]")
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

type listTableRow struct {
	cells []string
	task  map[string]any
}

func printTaskGroup(out, colorTarget io.Writer, status string, tasks []map[string]any, labelNames, packageNames map[string]string, showAssignees, showReviewers bool, width int) (effortEstimate, error) {
	style := "1;32"
	if status == "released" {
		style = "1;36"
	}
	fmt.Fprintln(out, colorText(colorTarget, style, displayStatus(status)))
	headers := []string{"NUMBER", "PRIORITY", "TYPE", "COMPLEXITY", "SP", "HOURS", "PACKAGE", "LABELS"}
	if showAssignees {
		headers = append(headers, "ASSIGNEES")
	}
	if showReviewers {
		headers = append(headers, "REVIEWERS")
	}
	headers = append(headers, "TITLE")
	var total effortEstimate
	rows := make([]listTableRow, 0, len(tasks))
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
		row := []string{tableCell(number), tableCell(currentTask["priority"]), tableCell(currentTask["type"]), complexity, storyPoints, hours, pkg, labels}
		if showAssignees {
			assignees := assigneesDisplay(currentTask["assignees"])
			if assignees == "" {
				assignees = "-"
			}
			row = append(row, tableCell(assignees))
		}
		if showReviewers {
			reviewers := assigneesDisplay(currentTask["reviewers"])
			if reviewers == "" {
				reviewers = "-"
			}
			row = append(row, tableCell(reviewers))
		}
		row = append(row, tableCell(currentTask["title"]))
		rows = append(rows, listTableRow{cells: row, task: currentTask})
	}
	if width > 0 {
		if err := writeResponsiveListTable(out, colorTarget, headers, rows, width, status); err != nil {
			return effortEstimate{}, err
		}
	} else if err := writeFullListTable(out, colorTarget, headers, rows, status); err != nil {
		return effortEstimate{}, err
	}
	fmt.Fprintf(out, "Subtotal: %d SP, %d hours\n", total.StoryPoints, total.Hours)
	return total, nil
}

func writeFullListTable(out, colorTarget io.Writer, headers []string, rows []listTableRow, status string) error {
	var table bytes.Buffer
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(writer, strings.Join(row.cells, "\t")); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(table.String(), "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			currentTask := rows[i-1].task
			if stringField(currentTask, "type") == "release" || status == "released" {
				line = colorText(colorTarget, "36", line)
			}
		}
		fmt.Fprintln(out, line)
	}
	return nil
}

func writeResponsiveListTable(out, colorTarget io.Writer, headers []string, rows []listTableRow, width int, status string) error {
	columns, widths := responsiveListColumns(headers, rows, width)
	if len(columns) == 0 {
		return errors.New("could not choose columns for task list")
	}
	titleIndex := len(headers) - 1
	for _, row := range append([]listTableRow{{cells: headers}}, rows...) {
		lines := []string{""}
		if row.task == nil {
			lines[0] = renderResponsiveListLine(row.cells, columns, widths, titleIndex, truncateListCell(row.cells[titleIndex], widths[len(widths)-1]))
		} else {
			title := row.cells[titleIndex]
			titleWidth := widths[len(widths)-1]
			lines = wrapTaskViewValue(title, titleWidth)
			if len(lines) == 0 {
				lines = []string{""}
			}
			lines[0] = renderResponsiveListLine(row.cells, columns, widths, titleIndex, lines[0])
			indent := responsiveListTitleIndent(widths)
			for index := 1; index < len(lines); index++ {
				lines[index] = strings.Repeat(" ", indent) + lines[index]
			}
		}
		for _, line := range lines {
			if row.task != nil && (stringField(row.task, "type") == "release" || status == "released") {
				line = colorText(colorTarget, "36", line)
			}
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func responsiveListColumns(headers []string, rows []listTableRow, width int) ([]int, []int) {
	if len(headers) == 0 {
		return nil, nil
	}
	columnWidths := make([]int, len(headers))
	for column, header := range headers {
		columnWidths[column] = utf8.RuneCountInString(header)
		for _, row := range rows {
			if column < len(row.cells) {
				if valueWidth := utf8.RuneCountInString(row.cells[column]); valueWidth > columnWidths[column] {
					columnWidths[column] = valueWidth
				}
			}
		}
	}

	columns := make([]int, len(headers))
	for index := range columns {
		columns[index] = index
	}
	titleIndex := len(headers) - 1
	const padding = 2
	const minimumTitleWidth = 12
	dropOrder := []string{"REVIEWERS", "ASSIGNEES", "LABELS", "PACKAGE", "HOURS", "SP", "TYPE", "COMPLEXITY", "PRIORITY"}
	for _, drop := range dropOrder {
		if responsiveListRequiredWidth(columns, columnWidths, titleIndex, minimumTitleWidth, padding) <= width {
			break
		}
		for position, column := range columns {
			if headers[column] == drop {
				columns = append(columns[:position], columns[position+1:]...)
				break
			}
		}
	}
	nonTitleWidth := 0
	for _, column := range columns {
		if column != titleIndex {
			nonTitleWidth += columnWidths[column]
		}
	}
	titleWidth := width - nonTitleWidth - padding*(len(columns)-1)
	if titleWidth < 1 {
		titleWidth = 1
	}
	widths := make([]int, len(columns))
	for position, column := range columns {
		if column == titleIndex {
			widths[position] = titleWidth
		} else {
			widths[position] = columnWidths[column]
		}
	}
	return columns, widths
}

func responsiveListRequiredWidth(columns, columnWidths []int, titleIndex, minimumTitleWidth, padding int) int {
	total := padding * (len(columns) - 1)
	for _, column := range columns {
		if column == titleIndex {
			total += minimumTitleWidth
		} else {
			total += columnWidths[column]
		}
	}
	return total
}

func renderResponsiveListLine(cells []string, columns, widths []int, titleIndex int, title string) string {
	var line strings.Builder
	for position, column := range columns {
		value := cells[column]
		if column == titleIndex {
			value = title
		}
		line.WriteString(value)
		if position < len(columns)-1 {
			line.WriteString(strings.Repeat(" ", max(0, widths[position]-utf8.RuneCountInString(value))+2))
		}
	}
	return line.String()
}

func responsiveListTitleIndent(widths []int) int {
	if len(widths) < 2 {
		return 0
	}
	indent := 2 * (len(widths) - 1)
	for _, width := range widths[:len(widths)-1] {
		indent += width
	}
	return indent
}

func truncateListCell(value string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
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
