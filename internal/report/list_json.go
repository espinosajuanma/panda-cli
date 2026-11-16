package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/task"
)

type ListJSONDocument struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	Groups         []ListJSONGroup       `json:"groups"`
	Totals         ListJSONTotals        `json:"totals"`
	AssigneeTotals []AssigneeEffortTotal `json:"assigneeTotals"`
}

type ListJSONGroup struct {
	Status string         `json:"status"`
	Tasks  []ListJSONTask `json:"tasks"`
	Totals ListJSONTotals `json:"totals"`
}

type ListJSONTask struct {
	Task        map[string]any `json:"task"`
	StoryPoints *int           `json:"storyPoints"`
	Hours       *int           `json:"hours"`
	Package     *string        `json:"package"`
	Labels      []string       `json:"labels"`
	Assignees   []string       `json:"assignees"`
	Reviewers   []string       `json:"reviewers"`
}

type ListJSONTotals struct {
	Tasks       int `json:"tasks"`
	StoryPoints int `json:"storyPoints"`
	Hours       int `json:"hours"`
}

func RenderListJSON(doc ListReport, out io.Writer) error {
	document := ListJSONDocument{
		SchemaVersion:  1,
		Groups:         make([]ListJSONGroup, 0, len(doc.Groups)),
		AssigneeTotals: append([]AssigneeEffortTotal{}, doc.AssigneeTotals...),
		Totals: ListJSONTotals{
			StoryPoints: doc.Total.StoryPoints,
			Hours:       doc.Total.Hours,
		},
	}
	for _, group := range doc.Groups {
		jsonGroup := ListJSONGroup{
			Status: group.Status,
			Tasks:  make([]ListJSONTask, 0, len(group.Entries)),
			Totals: ListJSONTotals{
				StoryPoints: group.Totals.StoryPoints,
				Hours:       group.Totals.Hours,
			},
		}
		for _, entry := range group.Entries {
			item := ListJSONTask{
				Task:      entry.Metadata,
				Labels:    relationshipNames(entry.Metadata["labels"], doc.LabelNames),
				Assignees: relationshipNames(entry.Metadata["assignees"], nil),
				Reviewers: relationshipNames(entry.Metadata["reviewers"], nil),
			}
			if effort, ok := task.EffortFromMap(entry.Metadata); ok {
				storyPoints, hours := effort.StoryPoints, effort.Hours
				item.StoryPoints, item.Hours = &storyPoints, &hours
			}
			if name := RelationshipDisplayNameWithCatalog(entry.Metadata["package"], doc.PackageNames); name != "" {
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
		if name := RelationshipDisplayNameWithCatalog(item, catalogNames); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}
