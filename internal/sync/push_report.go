package sync

import (
	"fmt"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
)

type PushOptions struct {
	Selectors []string
	JSON      bool
}

func recordTaskProblem(reportDoc *report.PushReport, problems *[]string, result *report.PushTaskResult, outcome, message string) {
	problem := fmt.Sprintf("%s: %s", result.Selector, message)
	*problems = append(*problems, problem)
	reportDoc.Errors = append(reportDoc.Errors, problem)
	result.Outcome = outcome
	result.Message = message
	reportDoc.Tasks = append(reportDoc.Tasks, *result)
}

func catalogPushResult(plan *CatalogPushPlan, changed bool) report.PushCatalogResult {
	result := report.PushCatalogResult{Changed: changed}
	if plan != nil {
		result.Created = plan.Created
		result.Updated = plan.Updated
		result.Deleted = plan.Deleted
	}
	return result
}

func updatePushTaskIdentity(result *report.PushTaskResult, metadata map[string]any) {
	updated := report.NewPushTaskResult(metadata, result.Selector)
	result.ID = updated.ID
	result.Number = updated.Number
	result.Title = updated.Title
}

func safeName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else if r == ' ' {
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
