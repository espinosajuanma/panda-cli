package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
)

type AssigneeEffortTotal struct {
	Assignee    string `json:"assignee"`
	Tasks       int    `json:"tasks"`
	StoryPoints int    `json:"storyPoints"`
	Hours       int    `json:"hours"`
}

func SummarizeAssigneeEffort(tasks []map[string]any) ([]AssigneeEffortTotal, int) {
	totals := make(map[string]*AssigneeEffortTotal)
	sharedTasks := 0
	for _, metadata := range tasks {
		effort, knownEffort := task.EffortFromMap(metadata)
		assignees := make(map[string]string)
		items, _ := metadata["assignees"].([]any)
		for _, item := range items {
			name := RelationshipDisplayName(item)
			if name == "" {
				continue
			}
			key := jsonutil.RelationshipID(item)
			if key == "" {
				key = strings.ToLower(name)
			}
			assignees[key] = name
		}
		if len(assignees) == 0 {
			assignees[""] = "Unassigned"
		}
		if len(assignees) > 1 {
			sharedTasks++
		}
		for key, name := range assignees {
			total := totals[key]
			if total == nil {
				total = &AssigneeEffortTotal{Assignee: name}
				totals[key] = total
			}
			total.Tasks++
			if knownEffort {
				total.StoryPoints += effort.StoryPoints
				total.Hours += effort.Hours
			}
		}
	}
	result := make([]AssigneeEffortTotal, 0, len(totals))
	for _, total := range totals {
		result = append(result, *total)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StoryPoints != result[j].StoryPoints {
			return result[i].StoryPoints > result[j].StoryPoints
		}
		return strings.ToLower(result[i].Assignee) < strings.ToLower(result[j].Assignee)
	})
	return result, sharedTasks
}

func RenderAssigneeEffortSummary(out, colorTarget io.Writer, title string, totals []AssigneeEffortTotal, sharedTasks int) error {
	if len(totals) == 0 {
		return nil
	}
	width := 0
	if ui.IsTerminal(colorTarget) {
		width, _, _ = ui.TerminalSize(colorTarget)
	}
	return RenderAssigneeEffortSummaryAtWidth(out, colorTarget, title, totals, sharedTasks, width)
}

func RenderAssigneeEffortSummaryAtWidth(out, colorTarget io.Writer, title string, totals []AssigneeEffortTotal, sharedTasks, width int) error {
	fmt.Fprintf(out, "\n%s\n", ui.ColorText(colorTarget, "1;32", title))
	if width > 0 && width < 56 {
		for _, total := range totals {
			nameWidth := max(1, width-2)
			for _, line := range WrapValue(total.Assignee, nameWidth) {
				fmt.Fprintf(out, "  %s\n", line)
			}
			if width < 36 {
				fmt.Fprintf(out, "    %dt · %d SP · %dh\n", total.Tasks, total.StoryPoints, total.Hours)
			} else {
				fmt.Fprintf(out, "    %d tasks · %d SP · %d hours\n", total.Tasks, total.StoryPoints, total.Hours)
			}
		}
		if sharedTasks > 0 {
			for _, line := range WrapValue("Shared-task effort counts in full for each assignee.", max(1, width)) {
				fmt.Fprintln(out, line)
			}
		}
		return nil
	}
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ASSIGNEE\tTASKS\tSP\tHOURS"); err != nil {
		return err
	}
	for _, total := range totals {
		if _, err := fmt.Fprintf(writer, "%s\t%d\t%d\t%d\n", TableCell(total.Assignee), total.Tasks, total.StoryPoints, total.Hours); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if sharedTasks > 0 {
		fmt.Fprintln(out, "Effort for shared tasks is counted in full for each assignee.")
	}
	return nil
}
