package app

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
)

type pushOptions struct {
	selectors []string
	json      bool
}

type pushReport = report.PushReport

type pushTaskResult = report.PushTaskResult

type pushCatalogResults = report.PushCatalogResults

type pushCatalogResult = report.PushCatalogResult

type pushSummary = report.PushSummary

func parsePushOptions(args []string) (pushOptions, error) {
	var options pushOptions
	for _, arg := range args {
		if arg == "--json" {
			options.json = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return pushOptions{}, errors.New("usage: panda push [taskNumber|draft-N ...] [--json]")
		}
		options.selectors = append(options.selectors, arg)
	}
	return options, nil
}

func renderPushJSON(doc pushReport, out io.Writer) error {
	return report.RenderPushJSON(doc, out)
}

func newPushTaskResult(metadata map[string]any, fallback string) pushTaskResult {
	return report.NewPushTaskResult(metadata, fallback)
}

func recordTaskProblem(report *pushReport, problems *[]string, result *pushTaskResult, outcome, message string) {
	problem := fmt.Sprintf("%s: %s", result.Selector, message)
	*problems = append(*problems, problem)
	report.Errors = append(report.Errors, problem)
	result.Outcome = outcome
	result.Message = message
	report.Tasks = append(report.Tasks, *result)
}

func catalogPushResult(plan *catalogPushPlan, changed bool) pushCatalogResult {
	result := pushCatalogResult{Changed: changed}
	if plan != nil {
		result.Created = plan.Created
		result.Updated = plan.Updated
		result.Deleted = plan.Deleted
	}
	return result
}

func updatePushTaskIdentity(result *pushTaskResult, metadata map[string]any) {
	updated := newPushTaskResult(metadata, result.Selector)
	result.ID = updated.ID
	result.Number = updated.Number
	result.Title = updated.Title
}
