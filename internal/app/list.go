package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type listOptions struct {
	statusFilter    map[string]bool
	statusSpecified bool
	allStatuses     bool
	noComplexity    bool
	noDescription   bool
	noPackage       bool
	noLabels        bool
	assignees       []string
	reviewers       []string
	json            bool
	wide            bool
}

type effortEstimate = task.Effort

type listTaskEntry = report.ListEntry

type listGroup = report.ListGroup

type listReport = report.ListReport

type listJSONDocument = report.ListJSONDocument

type listJSONTotals = report.ListJSONTotals

func listTasks(ws *workspace.Workspace, args []string, out io.Writer) error {
	return listTasksForUser(ws, args, "", out)
}

func listTasksForUser(ws *workspace.Workspace, args []string, userEmail string, out io.Writer) error {
	options, err := parseListOptions(args)
	if err != nil {
		return err
	}
	if options.json && options.wide {
		return errors.New("--wide cannot be combined with --json")
	}
	doc, err := collectListReportForUser(ws, options, userEmail)
	if err != nil {
		return err
	}
	if options.json {
		return report.RenderListJSON(doc, out)
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		return report.RenderList(doc, rendered, out, options.wide)
	})
}

func collectListReportForUser(ws *workspace.Workspace, options listOptions, userEmail string) (listReport, error) {
	var assigneeFilter, reviewerFilter relationFilter
	if len(options.assignees) > 0 || len(options.reviewers) > 0 {
		var members []map[string]any
		available := false
		if relationSelectorsNeedMembers(options.assignees) || relationSelectorsNeedMembers(options.reviewers) {
			var err error
			members, available, err = projectMembersIfAvailable(ws)
			if err != nil {
				return listReport{}, err
			}
		}
		var err error
		assigneeFilter, err = resolveRelationFilter(members, available, options.assignees, userEmail, "assignee")
		if err != nil {
			return listReport{}, err
		}
		reviewerFilter, err = resolveRelationFilter(members, available, options.reviewers, userEmail, "reviewer")
		if err != nil {
			return listReport{}, err
		}
	}
	labelsCatalog, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return listReport{}, err
	}
	packagesCatalog, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return listReport{}, err
	}
	doc := listReport{
		Groups:        make([]listGroup, 0),
		LabelNames:    catalogNameIndex(labelCatalog, labelsCatalog),
		PackageNames:  catalogNameIndex(packageCatalog, packagesCatalog),
		ShowAssignees: len(options.assignees) > 0,
		ShowReviewers: len(options.reviewers) > 0,
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return listReport{}, err
	}
	grouped := make(map[string][]listTaskEntry)
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return listReport{}, fmt.Errorf("read task metadata in %s: %w", filepath.Base(dir), err)
		}
		status := stringField(metadata, "status")
		if status == "" {
			status = "new"
		}
		if !options.allStatuses && !options.statusFilter[status] {
			continue
		}
		if options.noComplexity && !isEmptyField(metadata["complexity"]) {
			continue
		}
		if options.noDescription {
			description, err := os.ReadFile(filepath.Join(dir, "description.md"))
			if err != nil && !os.IsNotExist(err) {
				return listReport{}, fmt.Errorf("read task description in %s: %w", filepath.Base(dir), err)
			}
			if err == nil && strings.TrimSpace(string(description)) != "" {
				continue
			}
		}
		if options.noPackage && hasPackageAssignment(metadata["package"]) {
			continue
		}
		if options.noLabels && hasLabelAssignments(metadata["labels"]) {
			continue
		}
		if !assigneeFilter.matches(metadata["assignees"]) || !reviewerFilter.matches(metadata["reviewers"]) {
			continue
		}
		grouped[status] = append(grouped[status], listTaskEntry{Dir: dir, Metadata: metadata})
	}
	statuses := orderedTaskStatuses(grouped)
	var visibleTasks []map[string]any
	for _, status := range statuses {
		entries := grouped[status]
		sort.SliceStable(entries, func(i, j int) bool {
			return taskMetadataRankLess(entries[i].Metadata, entries[j].Metadata)
		})
		group := listGroup{Status: status, Entries: entries}
		for _, entry := range entries {
			if effort, ok := taskEffortFromMetadata(entry.Metadata); ok {
				group.Totals.StoryPoints += effort.StoryPoints
				group.Totals.Hours += effort.Hours
			}
			visibleTasks = append(visibleTasks, entry.Metadata)
		}
		doc.Total.StoryPoints += group.Totals.StoryPoints
		doc.Total.Hours += group.Totals.Hours
		doc.Groups = append(doc.Groups, group)
	}
	doc.AssigneeTotals, doc.SharedAssigneeTasks = summarizeAssigneeEffort(visibleTasks)
	return doc, nil
}

func parseListOptions(args []string) (listOptions, error) {
	options := listOptions{statusFilter: make(map[string]bool)}
	for _, status := range task.DefaultStatuses {
		options.statusFilter[status] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--current":
			addQuickStatusFilter(&options, task.CurrentStatuses)
		case arg == "--staging":
			addQuickStatusFilter(&options, []string{"staging"})
		case arg == "--released":
			addQuickStatusFilter(&options, []string{"released"})
		case arg == "--discarded":
			addQuickStatusFilter(&options, []string{"discarded"})
		case arg == "--all":
			options.statusSpecified = true
			options.allStatuses = true
		case arg == "--no-complexity":
			options.noComplexity = true
		case arg == "--no-description":
			options.noDescription = true
		case arg == "--no-package":
			options.noPackage = true
		case arg == "--no-labels":
			options.noLabels = true
		case arg == "--assignee" || arg == "--reviewer":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, listUsageError()
			}
			i++
			if arg == "--assignee" {
				if err := appendRelationSelectors(&options.assignees, args[i], "--assignee"); err != nil {
					return options, err
				}
			} else if err := appendRelationSelectors(&options.reviewers, args[i], "--reviewer"); err != nil {
				return options, err
			}
		case arg == "--json":
			options.json = true
		case arg == "--wide":
			options.wide = true
		case strings.HasPrefix(arg, "--assignee="):
			if err := appendRelationSelectors(&options.assignees, strings.TrimPrefix(arg, "--assignee="), "--assignee"); err != nil {
				return options, err
			}
		case strings.HasPrefix(arg, "--reviewer="):
			if err := appendRelationSelectors(&options.reviewers, strings.TrimPrefix(arg, "--reviewer="), "--reviewer"); err != nil {
				return options, err
			}
		case arg == "--status":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, listUsageError()
			}
			i++
			if err := addStatusFilter(&options, args[i]); err != nil {
				return options, err
			}
		case strings.HasPrefix(arg, "--status="):
			if err := addStatusFilter(&options, strings.TrimPrefix(arg, "--status=")); err != nil {
				return options, err
			}
		default:
			return options, listUsageError()
		}
	}
	return options, nil
}

func listUsageError() error {
	return errors.New("usage: panda list [--current] [--staging] [--released] [--discarded] [--all] [--status <status,...>] [--no-complexity] [--no-description] [--no-package] [--no-labels] [--assignee <member,...>] [--reviewer <member,...>] [--wide] [--json]")
}

func addQuickStatusFilter(options *listOptions, statuses []string) {
	beginExplicitStatusFilter(options)
	for _, status := range statuses {
		options.statusFilter[status] = true
	}
}

func beginExplicitStatusFilter(options *listOptions) {
	if options.statusSpecified {
		return
	}
	options.statusSpecified = true
	options.statusFilter = make(map[string]bool)
}

func addStatusFilter(options *listOptions, value string) error {
	beginExplicitStatusFilter(options)
	known := make(map[string]bool, len(task.StatusOrder))
	for _, status := range task.StatusOrder {
		known[status] = true
	}
	parts := strings.Split(value, ",")
	added := 0
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if !known[status] {
			return fmt.Errorf("unknown status %q; valid statuses are %s", status, strings.Join(task.StatusOrder, ", "))
		}
		options.statusFilter[status] = true
		added++
	}
	if added == 0 {
		return errors.New("--status requires one or more comma-separated statuses")
	}
	options.statusSpecified = true
	return nil
}

func orderedTaskStatuses(groups map[string][]listTaskEntry) []string {
	statuses := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, status := range task.StatusOrder {
		if len(groups[status]) > 0 {
			statuses = append(statuses, status)
			seen[status] = true
		}
	}
	var remaining []string
	for status := range groups {
		if !seen[status] {
			remaining = append(remaining, status)
		}
	}
	sort.Strings(remaining)
	return append(statuses, remaining...)
}

func taskMetadataRankLess(left, right map[string]any) bool {
	leftRank, rightRank := stringField(left, "rank"), stringField(right, "rank")
	if leftRank == "" || rightRank == "" {
		if leftRank != rightRank {
			return leftRank != ""
		}
	} else if leftRank != rightRank {
		return leftRank < rightRank
	}
	return compareTaskNumbers(left["number"], right["number"]) < 0
}

func compareTaskNumbers(left, right any) int {
	leftNumber, leftErr := strconv.ParseFloat(fmt.Sprint(left), 64)
	rightNumber, rightErr := strconv.ParseFloat(fmt.Sprint(right), 64)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}
