package app

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/task"
)

type listJSONDocument struct {
	SchemaVersion int             `json:"schemaVersion"`
	Groups        []listJSONGroup `json:"groups"`
	Totals        listJSONTotals  `json:"totals"`
}

type listJSONGroup struct {
	Status string         `json:"status"`
	Tasks  []listJSONTask `json:"tasks"`
	Totals listJSONTotals `json:"totals"`
}

type listJSONTask struct {
	Task        map[string]any `json:"task"`
	StoryPoints *int           `json:"storyPoints"`
	Hours       *int           `json:"hours"`
	Package     *string        `json:"package"`
	Labels      []string       `json:"labels"`
	Assignees   []string       `json:"assignees"`
	Reviewers   []string       `json:"reviewers"`
}

type listJSONTotals struct {
	Tasks       int `json:"tasks"`
	StoryPoints int `json:"storyPoints"`
	Hours       int `json:"hours"`
}

func renderListJSON(report listReport, out io.Writer) error {
	document := listJSONDocument{
		SchemaVersion: 1,
		Groups:        make([]listJSONGroup, 0, len(report.groups)),
		Totals: listJSONTotals{
			StoryPoints: report.total.StoryPoints,
			Hours:       report.total.Hours,
		},
	}
	for _, group := range report.groups {
		jsonGroup := listJSONGroup{
			Status: group.status,
			Tasks:  make([]listJSONTask, 0, len(group.entries)),
			Totals: listJSONTotals{
				StoryPoints: group.totals.StoryPoints,
				Hours:       group.totals.Hours,
			},
		}
		for _, entry := range group.entries {
			item := listJSONTask{
				Task:      entry.metadata,
				Labels:    relationshipNames(entry.metadata["labels"], report.labelNames),
				Assignees: relationshipNames(entry.metadata["assignees"], nil),
				Reviewers: relationshipNames(entry.metadata["reviewers"], nil),
			}
			if effort, ok := task.ComplexityEffort[strings.ToLower(strings.TrimSpace(stringField(entry.metadata, "complexity")))]; ok {
				storyPoints, hours := effort.StoryPoints, effort.Hours
				item.StoryPoints, item.Hours = &storyPoints, &hours
			}
			if name := relationshipDisplayNameWithCatalog(entry.metadata["package"], report.packageNames); name != "" {
				item.Package = &name
			}
			jsonGroup.Tasks = append(jsonGroup.Tasks, item)
		}
		jsonGroup.Totals.Tasks = len(jsonGroup.Tasks)
		document.Totals.Tasks += jsonGroup.Totals.Tasks
		document.Groups = append(document.Groups, jsonGroup)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}

func relationshipNames(value any, catalogNames map[string]string) []string {
	items, _ := value.([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := relationshipDisplayNameWithCatalog(item, catalogNames); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}
