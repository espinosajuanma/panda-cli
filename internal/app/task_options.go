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
	"time"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type labelOption struct {
	name string
	add  bool
}

type taskFieldOptions struct {
	fields              map[string]string
	provided            map[string]bool
	assignees           []string
	assigneesProvided   bool
	labels              []labelOption
	descriptionFile     string
	descriptionProvided bool
	supplied            bool
}

func parseTaskFieldOptions(args []string) (taskFieldOptions, []string, error) {
	options := taskFieldOptions{fields: make(map[string]string), provided: make(map[string]bool)}
	var positional []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "--") {
			positional = append(positional, arg)
			continue
		}
		name, value, hasInlineValue := strings.Cut(arg, "=")
		if !hasInlineValue {
			name = arg
		}
		consumeValue := func() (string, error) {
			if hasInlineValue {
				return value, nil
			}
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", fmt.Errorf("%s requires a value", name)
			}
			index++
			return args[index], nil
		}
		switch name {
		case "--title", "--type", "--priority", "--complexity", "--package":
			field := strings.TrimPrefix(name, "--")
			if options.provided[field] {
				return options, nil, fmt.Errorf("%s may only be supplied once", name)
			}
			fieldValue, err := consumeValue()
			if err != nil {
				return options, nil, err
			}
			options.fields[field] = fieldValue
			options.provided[field] = true
			options.supplied = true
		case "--assignee":
			fieldValue, err := consumeValue()
			if err != nil {
				return options, nil, err
			}
			options.assigneesProvided = true
			options.assignees = append(options.assignees, strings.Split(fieldValue, ",")...)
			options.supplied = true
		case "--label", "--add-label", "--remove-label":
			fieldValue, err := consumeValue()
			if err != nil {
				return options, nil, err
			}
			options.labels = append(options.labels, labelOption{name: fieldValue, add: name != "--remove-label"})
			options.supplied = true
		case "--description-file":
			if options.descriptionProvided {
				return options, nil, errors.New("--description-file may only be supplied once")
			}
			fieldValue, err := consumeValue()
			if err != nil {
				return options, nil, err
			}
			if strings.TrimSpace(fieldValue) == "" {
				return options, nil, errors.New("--description-file requires a file path or - for stdin")
			}
			options.descriptionFile = fieldValue
			options.descriptionProvided = true
			options.supplied = true
		default:
			return options, nil, fmt.Errorf("unknown task option %q", name)
		}
	}
	return options, positional, nil
}

func (options taskFieldOptions) hasMetadataFields() bool {
	return len(options.fields) > 0 || options.assigneesProvided || len(options.labels) > 0
}

func applyTaskFieldOptions(ws *workspace.Workspace, metadata map[string]any, options taskFieldOptions, in io.Reader) (map[string]any, []byte, error) {
	updated := cloneMap(metadata)
	for _, field := range []string{"title", "type", "priority", "complexity"} {
		value, provided := options.fields[field]
		if !provided {
			continue
		}
		value = strings.TrimSpace(value)
		if field == "title" && value == "" {
			return nil, nil, errors.New("--title cannot be empty")
		}
		if field == "complexity" && isNoneSelector(value) {
			delete(updated, field)
			continue
		}
		var allowed []string
		switch field {
		case "type":
			allowed = []string{"feature", "bug", "other", "release"}
		case "priority":
			allowed = []string{"hotfix", "high", "medium", "low"}
		case "complexity":
			allowed = []string{"xs", "s", "m", "l", "xl", "xxl"}
		}
		if field != "title" && !contains(allowed, value) {
			return nil, nil, fmt.Errorf("invalid %s %q; choose one of %s or none", field, value, strings.Join(allowed, ", "))
		}
		updated[field] = value
	}

	if packageName, provided := options.fields["package"]; provided {
		packageName = strings.TrimSpace(packageName)
		if isNoneSelector(packageName) {
			delete(updated, "package")
		} else {
			if stringField(updated, "type") == "release" {
				return nil, nil, errors.New("release tasks cannot be assigned to a package")
			}
			packages, err := readCatalog(ws, packageCatalog)
			if err != nil {
				return nil, nil, err
			}
			match := findCatalogRecordByName(packages, packageCatalog, packageName)
			if match == nil {
				return nil, nil, fmt.Errorf("package %q not found in the local project catalog; run `panda pull` first", packageName)
			}
			updated["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
		}
	}

	if options.assigneesProvided {
		if len(options.assignees) == 1 && isNoneSelector(options.assignees[0]) {
			updated["assignees"] = []any{}
		} else {
			for _, assignee := range options.assignees {
				if isNoneSelector(assignee) {
					return nil, nil, errors.New("--assignee none cannot be combined with member names")
				}
			}
			allMembers, err := projectMembers(ws)
			if err != nil {
				return nil, nil, err
			}
			members := eligibleProjectMembers(allMembers)
			selected := make(map[string]map[string]any)
			for _, assignee := range options.assignees {
				assignee = strings.TrimSpace(assignee)
				match := findProjectMember(members, assignee)
				if match == nil {
					if findProjectMember(allMembers, assignee) != nil {
						return nil, nil, fmt.Errorf("project member %q is not eligible for assignment; eligible roles are %s", assignee, strings.Join(assignableTeamRoles, ", "))
					}
					return nil, nil, fmt.Errorf("project member %q not found; run `panda pull` to refresh members", assignee)
				}
				user := memberUser(match)
				id := relationshipID(user)
				if id == "" {
					return nil, nil, fmt.Errorf("project member %q has no user ID", assignee)
				}
				selected[id] = map[string]any{"id": id, "label": memberDisplayName(match)}
			}
			assignees := make([]any, 0, len(selected))
			for _, assignee := range selected {
				assignees = append(assignees, assignee)
			}
			sort.Slice(assignees, func(i, j int) bool {
				left, _ := assignees[i].(map[string]any)
				right, _ := assignees[j].(map[string]any)
				return strings.ToLower(relationshipDisplayName(left)) < strings.ToLower(relationshipDisplayName(right))
			})
			updated["assignees"] = assignees
		}
	}

	if len(options.labels) > 0 {
		clearLabels := len(options.labels) == 1 && options.labels[0].add && isNoneSelector(options.labels[0].name)
		for _, operation := range options.labels {
			if isNoneSelector(operation.name) && !clearLabels {
				return nil, nil, errors.New("label none must be used alone to clear all labels")
			}
		}
		if clearLabels {
			updated["labels"] = []any{}
		} else {
			labels, err := readCatalog(ws, labelCatalog)
			if err != nil {
				return nil, nil, err
			}
			current, _ := updated["labels"].([]any)
			result := append([]any(nil), current...)
			for _, operation := range options.labels {
				name := strings.TrimSpace(operation.name)
				match := findCatalogRecordByName(labels, labelCatalog, name)
				if match == nil {
					return nil, nil, fmt.Errorf("label %q not found in the local project catalog; run `panda pull` first", name)
				}
				id := catalogRecordID(match)
				if operation.add {
					alreadyAssigned := false
					for _, relation := range result {
						if relationshipID(relation) == id {
							alreadyAssigned = true
							break
						}
					}
					if !alreadyAssigned {
						result = append(result, map[string]any{"id": id, "label": catalogRecordName(labelCatalog, match)})
					}
					continue
				}
				filtered := result[:0]
				for _, relation := range result {
					if relationshipID(relation) != id {
						filtered = append(filtered, relation)
					}
				}
				result = filtered
			}
			if result == nil {
				result = []any{}
			}
			updated["labels"] = result
		}
	}

	var description []byte
	if options.descriptionProvided {
		var err error
		description, err = readTaskDescription(options.descriptionFile, in)
		if err != nil {
			return nil, nil, err
		}
	}
	return updated, description, nil
}

func readTaskDescription(path string, in io.Reader) ([]byte, error) {
	var data []byte
	var err error
	if path == "-" {
		if in == nil {
			return nil, errors.New("cannot read task description from stdin")
		}
		data, err = io.ReadAll(in)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read description %q: %w", path, err)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("description %q is not valid UTF-8", path)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, errors.New("task description cannot be empty")
	}
	return data, nil
}

func createTaskWithOptions(ws *workspace.Workspace, projectID, projectName string, options taskFieldOptions, positional []string, in io.Reader, out io.Writer) error {
	title, titleProvided := options.fields["title"]
	if titleProvided && len(positional) > 0 {
		return errors.New("provide the task title with either --title or positional text, not both")
	}
	if !titleProvided {
		title = strings.TrimSpace(strings.Join(positional, " "))
		if title == "" {
			return errors.New("noninteractive task creation requires --title (or positional title text)")
		}
		options.fields["title"] = title
		options.provided["title"] = true
	}
	localID := "local-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	metadata := map[string]any{
		"localId":   localID,
		"project":   map[string]any{"id": projectID, "label": projectName},
		"title":     strings.TrimSpace(title),
		"type":      "feature",
		"priority":  "medium",
		"status":    "new",
		"labels":    []any{},
		"assignees": []any{},
		"reviewers": []any{},
	}
	updated, description, err := applyTaskFieldOptions(ws, metadata, options, in)
	if err != nil {
		return err
	}
	dir := ws.TaskPath(localID)
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		return err
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), updated); err != nil {
		return err
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), description, 0o600); err != nil {
		return err
	}
	if err := ws.WriteState(localID, workspace.State{LocalCreated: true}); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created local task %s. Run `panda edit %s` to edit it and `panda push` to publish it.\n", localID, localID)
	return nil
}
