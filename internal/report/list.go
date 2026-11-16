package report

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
)

type ListEntry struct {
	Dir      string
	Metadata map[string]any
}

type ListGroup struct {
	Status  string
	Entries []ListEntry
	Totals  task.Effort
}

type ListReport struct {
	Groups              []ListGroup
	Total               task.Effort
	AssigneeTotals      []AssigneeEffortTotal
	SharedAssigneeTasks int
	LabelNames          map[string]string
	PackageNames        map[string]string
	ShowAssignees       bool
	ShowReviewers       bool
}

func RenderList(doc ListReport, out, colorTarget io.Writer, wide bool) error {
	if len(doc.Groups) == 0 {
		fmt.Fprintln(out, "No tasks match the filters.")
		return nil
	}
	terminal := ui.IsTerminal(colorTarget)
	terminalWidth := 0
	if terminal && !wide {
		terminalWidth, _, _ = ui.TerminalSize(colorTarget)
	}
	width := ListTableWidth(wide, terminal, terminalWidth)
	for index, group := range doc.Groups {
		if index > 0 {
			fmt.Fprintln(out, ui.ColorText(colorTarget, "2", "----------------------------------------"))
		}
		metadata := make([]map[string]any, 0, len(group.Entries))
		for _, entry := range group.Entries {
			metadata = append(metadata, entry.Metadata)
		}
		_, err := PrintTaskGroup(out, colorTarget, group.Status, metadata, doc.LabelNames, doc.PackageNames, doc.ShowAssignees, doc.ShowReviewers, width)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "TOTAL (visible tasks): %d SP, %d hours\n", doc.Total.StoryPoints, doc.Total.Hours)
	return RenderAssigneeEffortSummary(out, colorTarget, "BY ASSIGNEE", doc.AssigneeTotals, doc.SharedAssigneeTasks)
}

func ListTableWidth(wide, terminal bool, terminalWidth int) int {
	if wide || !terminal {
		return 0
	}
	if terminalWidth < 1 {
		return 80
	}
	return terminalWidth
}

type listTableRow struct {
	cells []string
	task  map[string]any
}

func PrintTaskGroup(out, colorTarget io.Writer, status string, tasks []map[string]any, labelNames, packageNames map[string]string, showAssignees, showReviewers bool, width int) (task.Effort, error) {
	style := "1;32"
	if status == "released" {
		style = "1;36"
	}
	fmt.Fprintln(out, ui.ColorText(colorTarget, style, DisplayStatus(status)))
	showRelease := status == "released"
	headers := []string{"NUMBER", "PRIORITY", "TYPE", "COMPLEXITY", "SP", "HOURS", "PACKAGE"}
	if showRelease {
		headers = append(headers, "RELEASE")
	}
	headers = append(headers, "LABELS")
	if showAssignees {
		headers = append(headers, "ASSIGNEES")
	}
	if showReviewers {
		headers = append(headers, "REVIEWERS")
	}
	headers = append(headers, "TITLE")
	var total task.Effort
	rows := make([]listTableRow, 0, len(tasks))
	for _, currentTask := range tasks {
		number := jsonutil.StringField(currentTask, "number")
		if number == "" {
			number = task.Label(currentTask)
		}
		complexity := TableCell(currentTask["complexity"])
		if complexity == "" {
			complexity = "-"
		}
		storyPoints, hours := "-", "-"
		if estimate, ok := task.EffortFromMap(currentTask); ok {
			storyPoints = strconv.Itoa(estimate.StoryPoints)
			hours = strconv.Itoa(estimate.Hours)
			total.StoryPoints += estimate.StoryPoints
			total.Hours += estimate.Hours
		}
		pkg := RelationshipDisplayNameWithCatalog(currentTask["package"], packageNames)
		if pkg == "" {
			pkg = "-"
		}
		labels := LabelsDisplayWithCatalog(currentTask["labels"], labelNames)
		if labels == "" {
			labels = "-"
		}
		row := []string{TableCell(number), TableCell(currentTask["priority"]), TableCell(currentTask["type"]), complexity, storyPoints, hours, pkg}
		if showRelease {
			release := RelationshipDisplayName(currentTask["release"])
			if release == "" {
				release = "-"
			}
			row = append(row, TableCell(release))
		}
		row = append(row, labels)
		if showAssignees {
			assignees := AssigneesDisplay(currentTask["assignees"])
			if assignees == "" {
				assignees = "-"
			}
			row = append(row, TableCell(assignees))
		}
		if showReviewers {
			reviewers := AssigneesDisplay(currentTask["reviewers"])
			if reviewers == "" {
				reviewers = "-"
			}
			row = append(row, TableCell(reviewers))
		}
		row = append(row, TableCell(currentTask["title"]))
		rows = append(rows, listTableRow{cells: row, task: currentTask})
	}
	if width > 0 {
		if err := writeResponsiveListTable(out, colorTarget, headers, rows, width, status); err != nil {
			return task.Effort{}, err
		}
	} else if err := writeFullListTable(out, colorTarget, headers, rows, status); err != nil {
		return task.Effort{}, err
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
			if jsonutil.StringField(currentTask, "type") == "release" || status == "released" {
				line = ui.ColorText(colorTarget, "36", line)
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
			lines[0] = renderResponsiveListLine(row.cells, columns, widths, titleIndex, TruncateCell(row.cells[titleIndex], widths[len(widths)-1]))
		} else {
			title := row.cells[titleIndex]
			titleWidth := widths[len(widths)-1]
			lines = WrapValue(title, titleWidth)
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
			if row.task != nil && (jsonutil.StringField(row.task, "type") == "release" || status == "released") {
				line = ui.ColorText(colorTarget, "36", line)
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
	dropOrder := []string{"REVIEWERS", "ASSIGNEES", "LABELS", "PACKAGE", "RELEASE", "HOURS", "SP", "TYPE", "COMPLEXITY", "PRIORITY"}
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
