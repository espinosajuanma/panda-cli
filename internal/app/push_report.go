package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type pushOptions struct {
	selectors []string
	json      bool
}

type pushReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Tasks         []pushTaskResult   `json:"tasks"`
	Catalogs      pushCatalogResults `json:"catalogs"`
	Errors        []string           `json:"errors"`
	Summary       pushSummary        `json:"summary"`
}

type pushTaskResult struct {
	Selector      string   `json:"selector"`
	ID            string   `json:"id,omitempty"`
	Number        any      `json:"number,omitempty"`
	Title         string   `json:"title,omitempty"`
	RemoteCreated bool     `json:"remoteCreated,omitempty"`
	Outcome       string   `json:"outcome"`
	Actions       []string `json:"actions"`
	Message       string   `json:"message,omitempty"`
}

type pushCatalogResults struct {
	Labels   pushCatalogResult `json:"labels"`
	Packages pushCatalogResult `json:"packages"`
}

type pushCatalogResult struct {
	Changed bool `json:"changed"`
	Created int  `json:"created"`
	Updated int  `json:"updated"`
	Deleted int  `json:"deleted"`
}

type pushSummary struct {
	Pushed  int `json:"pushed"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

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

func renderPushJSON(report pushReport, out io.Writer) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func newPushTaskResult(metadata map[string]any, fallback string) pushTaskResult {
	selector := taskLabel(metadata)
	if selector == "local task" || selector == "" {
		selector = fallback
	}
	return pushTaskResult{
		Selector: selector,
		ID:       stringField(metadata, "id"),
		Number:   metadata["number"],
		Title:    stringField(metadata, "title"),
		Actions:  make([]string, 0),
	}
}

func (result *pushTaskResult) addAction(action string) {
	for _, existing := range result.Actions {
		if existing == action {
			return
		}
	}
	result.Actions = append(result.Actions, action)
}

func (report *pushReport) updateSummary() {
	report.Summary = pushSummary{Failed: len(report.Errors)}
	for _, task := range report.Tasks {
		if task.RemoteCreated {
			report.Summary.Created++
		}
		switch task.Outcome {
		case "created":
			report.Summary.Pushed++
		case "pushed":
			report.Summary.Updated++
			report.Summary.Pushed++
		case "skipped":
			report.Summary.Skipped++
		}
	}
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
		result.Created = plan.created
		result.Updated = plan.updated
		result.Deleted = plan.deleted
	}
	return result
}

func updatePushTaskIdentity(result *pushTaskResult, metadata map[string]any) {
	updated := newPushTaskResult(metadata, result.Selector)
	result.ID = updated.ID
	result.Number = updated.Number
	result.Title = updated.Title
}
