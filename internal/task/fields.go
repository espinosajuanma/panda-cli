package task

import (
	"fmt"
	"strings"
)

type Field string

const (
	FieldTitle      Field = "title"
	FieldType       Field = "type"
	FieldPriority   Field = "priority"
	FieldComplexity Field = "complexity"
	FieldPackage    Field = "package"
	FieldLabels     Field = "labels"
	FieldStatus     Field = "status"
	FieldAssignees  Field = "assignees"
	FieldReviewers  Field = "reviewers"
)

// EditableFields are the task metadata fields that participate in three-way sync.
var EditableFields = []string{
	string(FieldTitle), string(FieldType), string(FieldPriority), string(FieldComplexity),
	string(FieldPackage), string(FieldLabels), string(FieldStatus), string(FieldAssignees), string(FieldReviewers),
}

var StatusOrder = []string{"new", "toDo", "inProgress", "inReview", "completed", "staging", "released", "discarded"}
var DefaultStatuses = []string{"new", "toDo", "inProgress", "inReview", "completed"}
var CurrentStatuses = []string{"toDo", "inProgress", "inReview", "completed"}

type Effort struct {
	StoryPoints int
	Hours       int
}

var ComplexityEffort = map[string]Effort{
	"xs":  {StoryPoints: 1, Hours: 4},
	"s":   {StoryPoints: 2, Hours: 8},
	"m":   {StoryPoints: 3, Hours: 12},
	"l":   {StoryPoints: 5, Hours: 20},
	"xl":  {StoryPoints: 8, Hours: 32},
	"xxl": {StoryPoints: 13, Hours: 52},
}

var ComplexityOrder = []string{"xs", "s", "m", "l", "xl", "xxl"}

func ComplexityLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	effort, ok := ComplexityEffort[value]
	if !ok {
		return value
	}
	return fmt.Sprintf("%s — %d SP / %d hours", value, effort.StoryPoints, effort.Hours)
}
