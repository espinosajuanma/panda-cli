package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
)

type PushReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Tasks         []PushTaskResult   `json:"tasks"`
	Catalogs      PushCatalogResults `json:"catalogs"`
	Errors        []string           `json:"errors"`
	Summary       PushSummary        `json:"summary"`
}

type PushTaskResult struct {
	Selector      string   `json:"selector"`
	ID            string   `json:"id,omitempty"`
	Number        any      `json:"number,omitempty"`
	Title         string   `json:"title,omitempty"`
	RemoteCreated bool     `json:"remoteCreated,omitempty"`
	Outcome       string   `json:"outcome"`
	Actions       []string `json:"actions"`
	Message       string   `json:"message,omitempty"`
}

type PushCatalogResults struct {
	Labels   PushCatalogResult `json:"labels"`
	Packages PushCatalogResult `json:"packages"`
}

type PushCatalogResult struct {
	Changed bool `json:"changed"`
	Created int  `json:"created"`
	Updated int  `json:"updated"`
	Deleted int  `json:"deleted"`
}

type PushSummary struct {
	Pushed  int `json:"pushed"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func RenderPushJSON(report PushReport, out io.Writer) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func NewPushTaskResult(metadata map[string]any, fallback string) PushTaskResult {
	selector := task.Label(metadata)
	if selector == "local task" || selector == "" {
		selector = fallback
	}
	return PushTaskResult{
		Selector: selector,
		ID:       jsonutil.StringField(metadata, "id"),
		Number:   metadata["number"],
		Title:    jsonutil.StringField(metadata, "title"),
		Actions:  make([]string, 0),
	}
}

func (result *PushTaskResult) AddAction(action string) {
	for _, existing := range result.Actions {
		if existing == action {
			return
		}
	}
	result.Actions = append(result.Actions, action)
}

func (report *PushReport) UpdateSummary() {
	report.Summary = PushSummary{Failed: len(report.Errors)}
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
