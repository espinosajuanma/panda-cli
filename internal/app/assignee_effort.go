package app

import (
	"io"

	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/task"
)

type assigneeEffortTotal = report.AssigneeEffortTotal

func taskEffortFromMetadata(metadata map[string]any) (task.Effort, bool) {
	return task.EffortFromMap(metadata)
}

func summarizeAssigneeEffort(tasks []map[string]any) ([]assigneeEffortTotal, int) {
	return report.SummarizeAssigneeEffort(tasks)
}

func renderAssigneeEffortSummary(out, colorTarget io.Writer, title string, totals []assigneeEffortTotal, sharedTasks int) error {
	return report.RenderAssigneeEffortSummary(out, colorTarget, title, totals, sharedTasks)
}

func renderAssigneeEffortSummaryAtWidth(out, colorTarget io.Writer, title string, totals []assigneeEffortTotal, sharedTasks, width int) error {
	return report.RenderAssigneeEffortSummaryAtWidth(out, colorTarget, title, totals, sharedTasks, width)
}
