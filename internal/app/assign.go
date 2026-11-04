package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type assignOptions struct {
	statuses       []string
	statusFilter   map[string]bool
	priorities     []string
	priorityFilter map[string]bool
	types          []string
	typeFilter     map[string]bool
}

type assignmentTask struct {
	dir      string
	status   string
	metadata map[string]any
}

type assignmentInteraction interface {
	Action() (rune, error)
	SelectAssignees(options []selectorOption, selected map[string]bool) (map[string]bool, error)
}

type terminalAssignmentInteraction struct {
	in  io.Reader
	out io.Writer
}

func (interaction terminalAssignmentInteraction) Action() (rune, error) {
	return readTaskAction(interaction.in, interaction.out, "Enter choose assignees · d description · s skip · q quit")
}

func (interaction terminalAssignmentInteraction) SelectAssignees(options []selectorOption, selected map[string]bool) (map[string]bool, error) {
	return selectMultipleOptionsWithEscapeCancel(interaction.in, interaction.out, "Assignees · Space toggle · Enter save · Esc back", options, selected)
}

func assignTask(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return assignTaskByPerson(ws, args, out)
	}
	options, err := parseAssignOptions(args)
	if err != nil {
		return err
	}
	allTasks, releaseTasksSkipped, err := findAssignmentTasks(ws, options)
	if err != nil {
		return err
	}
	if len(allTasks) == 0 {
		fmt.Fprintf(out, "No tasks match statuses %s", strings.Join(options.statuses, ", "))
		if len(options.priorities) > 0 {
			fmt.Fprintf(out, ", priorities %s", strings.Join(options.priorities, ", "))
		}
		if len(options.types) > 0 {
			fmt.Fprintf(out, ", types %s", strings.Join(options.types, ", "))
		}
		fmt.Fprintln(out, ".")
		if releaseTasksSkipped {
			fmt.Fprintln(out, "Release tasks are skipped by default; pass `--type release` to include them.")
		}
		return nil
	}
	if !isInteractiveTerminal(in, out) {
		return errors.New("filtered `panda assign` requires an interactive terminal")
	}
	members, err := projectMembers(ws)
	if err != nil {
		return err
	}
	members = eligibleProjectMembers(members)
	if len(members) == 0 {
		return fmt.Errorf("no eligible project members found; eligible roles are %s", strings.Join(assignableTeamRoles, ", "))
	}
	return assignFilteredTasks(options, allTasks, members, terminalAssignmentInteraction{in: in, out: out}, out)
}

func parseAssignOptions(args []string) (assignOptions, error) {
	options := assignOptions{
		statusFilter:   make(map[string]bool),
		priorityFilter: make(map[string]bool),
		typeFilter:     make(map[string]bool),
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		name, value, inline := strings.Cut(arg, "=")
		if name != "--status" && name != "--priority" && name != "--type" {
			return options, assignUsageError()
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return options, fmt.Errorf("%s requires one or more values", name)
			}
			index++
			value = args[index]
		}
		parts := strings.Split(value, ",")
		added := 0
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item == "" {
				continue
			}
			var allowed []string
			var filter map[string]bool
			switch name {
			case "--status":
				allowed, filter = task.StatusOrder, options.statusFilter
			case "--priority":
				allowed, filter = assignPriorityOrder, options.priorityFilter
			case "--type":
				allowed, filter = estimateTypeOrder, options.typeFilter
			}
			if !contains(allowed, item) {
				return options, fmt.Errorf("unknown %s %q; valid values are %s", strings.TrimPrefix(name, "--"), item, strings.Join(allowed, ", "))
			}
			filter[item] = true
			added++
		}
		if added == 0 {
			return options, fmt.Errorf("%s requires one or more comma-separated values", name)
		}
	}
	if len(options.statusFilter) == 0 {
		return options, errors.New("filtered `panda assign` requires --status")
	}
	for _, status := range task.StatusOrder {
		if options.statusFilter[status] {
			options.statuses = append(options.statuses, status)
		}
	}
	for _, priority := range assignPriorityOrder {
		if options.priorityFilter[priority] {
			options.priorities = append(options.priorities, priority)
		}
	}
	for _, taskType := range estimateTypeOrder {
		if options.typeFilter[taskType] {
			options.types = append(options.types, taskType)
		}
	}
	return options, nil
}

var assignPriorityOrder = []string{"hotfix", "high", "medium", "low"}

func assignUsageError() error {
	return errors.New("usage: panda assign <taskNumber|draft-N> <person[,person...]|none> | panda assign --status <status,...> [--priority <priority,...>] [--type <type,...>]")
}

func findAssignmentTasks(ws *workspace.Workspace, options assignOptions) ([]assignmentTask, bool, error) {
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return nil, false, err
	}
	byStatus := make(map[string][]assignmentTask)
	releaseTasksSkipped := false
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return nil, false, fmt.Errorf("read task metadata in %s: %w", filepath.Base(dir), err)
		}
		status := stringField(metadata, "status")
		if status == "" {
			status = "new"
		}
		if !options.statusFilter[status] {
			continue
		}
		if len(options.priorityFilter) > 0 && !options.priorityFilter[stringField(metadata, "priority")] {
			continue
		}
		taskType := stringField(metadata, "type")
		if len(options.typeFilter) == 0 && taskType == "release" {
			releaseTasksSkipped = true
			continue
		}
		if len(options.typeFilter) > 0 && !options.typeFilter[taskType] {
			continue
		}
		byStatus[status] = append(byStatus[status], assignmentTask{dir: dir, status: status, metadata: metadata})
	}
	var selected []assignmentTask
	for _, status := range options.statuses {
		group := byStatus[status]
		sort.SliceStable(group, func(i, j int) bool {
			return taskMetadataRankLess(group[i].metadata, group[j].metadata)
		})
		selected = append(selected, group...)
	}
	return selected, releaseTasksSkipped, nil
}

func assignFilteredTasks(options assignOptions, candidates []assignmentTask, members []map[string]any, interaction assignmentInteraction, out io.Writer) error {
	updated, skipped := 0, 0
	quit := false
	for index := range candidates {
		current := &candidates[index]
		printAssignmentTaskHeader(out, *current, index+1, len(candidates))
		for {
			action, err := interaction.Action()
			if err != nil {
				return err
			}
			switch action {
			case rune(selectorEnter):
				choices, selected, relations := assigneePickerState(members, current.metadata)
				selection, err := interaction.SelectAssignees(choices, selected)
				if errors.Is(err, errSelectionEscaped) {
					continue
				}
				if errors.Is(err, errSelectionCanceled) {
					quit = true
					break
				}
				if err != nil {
					return err
				}
				updatedMetadata, changed, err := saveAssigneeSelection(current.dir, current.metadata, selection, relations)
				if err != nil {
					return err
				}
				current.metadata = updatedMetadata
				if changed {
					updated++
					fmt.Fprintf(out, "Saved assignees for %s: %s\n", taskLabel(current.metadata), displayedAssignees(current.metadata))
				} else {
					skipped++
					fmt.Fprintf(out, "No assignment changes for %s.\n", taskLabel(current.metadata))
				}
			case 'd', 'D':
				if err := showTaskDescriptionForTask(current.dir, current.metadata, out); err != nil {
					return err
				}
				continue
			case 's', 'S':
				skipped++
				fmt.Fprintf(out, "Skipped %s.\n", taskLabel(current.metadata))
			case 'q', 'Q', rune(selectorCancel):
				quit = true
			}
			break
		}
		if quit {
			break
		}
	}
	if quit {
		fmt.Fprint(out, "Stopped early. ")
	}
	fmt.Fprintf(out, "Assignment complete: %d updated, %d skipped.\n", updated, skipped)
	metadata := make([]map[string]any, 0, len(candidates))
	for _, current := range candidates {
		metadata = append(metadata, current.metadata)
	}
	totals, shared := summarizeAssigneeEffort(metadata)
	if err := renderAssigneeEffortSummary(out, out, "ASSIGNEE LOAD (MATCHING TASKS)", totals, shared); err != nil {
		return err
	}
	if updated > 0 {
		fmt.Fprintln(out, "Run `panda push` to publish these local changes.")
	}
	return nil
}

func printAssignmentTaskHeader(out io.Writer, current assignmentTask, index, total int) {
	identifier := stringField(current.metadata, "number")
	if identifier == "" {
		identifier = taskLabel(current.metadata)
	}
	title := tableCell(current.metadata["title"])
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(out, "\n%s\n", colorText(out, "1;32", fmt.Sprintf("#%s — %s", identifier, title)))
	details := []string{fmt.Sprintf("%d/%d", index, total), displayStatus(current.status)}
	if value := stringField(current.metadata, "type"); value != "" {
		details = append(details, value)
	}
	if value := stringField(current.metadata, "priority"); value != "" {
		details = append(details, value+" priority")
	}
	if effort, ok := taskEffortFromMetadata(current.metadata); ok {
		details = append(details, fmt.Sprintf("%d SP / %d hours", effort.StoryPoints, effort.Hours))
	} else {
		details = append(details, "effort not estimated")
	}
	assignees := displayedAssignees(current.metadata)
	fmt.Fprintf(out, "  %s\n  Current assignees: %s\n", strings.Join(details, " · "), assignees)
}

func displayedAssignees(metadata map[string]any) string {
	assignees := assigneesDisplay(metadata["assignees"])
	if assignees == "" {
		return "Unassigned"
	}
	return assignees
}

func assigneePickerState(members []map[string]any, metadata map[string]any) ([]selectorOption, map[string]bool, map[string]map[string]any) {
	sortedMembers := append([]map[string]any(nil), members...)
	sort.SliceStable(sortedMembers, func(i, j int) bool {
		return strings.ToLower(memberDisplayName(sortedMembers[i])) < strings.ToLower(memberDisplayName(sortedMembers[j]))
	})
	options := make([]selectorOption, 0, len(sortedMembers))
	relations := make(map[string]map[string]any, len(sortedMembers))
	selected := make(map[string]bool)
	seen := make(map[string]bool)
	for _, member := range sortedMembers {
		user := memberUser(member)
		id := relationshipID(user)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		relations[id] = map[string]any{"id": id, "label": memberDisplayName(member)}
		options = append(options, selectorOption{value: id, label: memberDisplayName(member) + " (" + stringField(member, "role") + ")"})
	}
	current, _ := metadata["assignees"].([]any)
	for _, item := range current {
		relation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := relationshipID(relation)
		if id == "" {
			continue
		}
		selected[id] = true
		if _, exists := relations[id]; !exists {
			relations[id] = cloneMap(relation)
			options = append(options, selectorOption{value: id, label: relationshipDisplayName(relation) + " (not in current roster)"})
		}
	}
	sort.SliceStable(options, func(i, j int) bool {
		return strings.ToLower(options[i].label) < strings.ToLower(options[j].label)
	})
	return options, selected, relations
}

func saveAssigneeSelection(dir string, metadata map[string]any, selected map[string]bool, relations map[string]map[string]any) (map[string]any, bool, error) {
	updated := cloneMap(metadata)
	assignments := make([]any, 0, len(selected))
	for id, isSelected := range selected {
		if !isSelected || id == "" {
			continue
		}
		if relation := relations[id]; relation != nil {
			assignments = append(assignments, cloneMap(relation))
		}
	}
	sort.Slice(assignments, func(i, j int) bool {
		return strings.ToLower(relationshipDisplayName(assignments[i])) < strings.ToLower(relationshipDisplayName(assignments[j]))
	})
	updated["assignees"] = assignments
	if sameAssigneeSelection(metadata["assignees"], assignments) {
		return metadata, false, nil
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), updated); err != nil {
		return nil, false, fmt.Errorf("save assignees for task %s: %w", taskLabel(metadata), err)
	}
	return updated, true, nil
}

func sameAssigneeSelection(left, right any) bool {
	keys := func(value any) map[string]bool {
		result := make(map[string]bool)
		items, _ := value.([]any)
		for _, item := range items {
			key := relationshipID(item)
			if key == "" {
				key = strings.ToLower(relationshipDisplayName(item))
			}
			if key != "" {
				result[key] = true
			}
		}
		return result
	}
	leftKeys, rightKeys := keys(left), keys(right)
	if len(leftKeys) != len(rightKeys) {
		return false
	}
	for key := range leftKeys {
		if !rightKeys[key] {
			return false
		}
	}
	return true
}
