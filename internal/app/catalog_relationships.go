package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

var assignableTeamRoles = []string{"teamLead", "developer", "qa"}

func projectMembers(ws *workspace.Workspace) ([]map[string]any, error) {
	members, available, err := projectMembersIfAvailable(ws)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, errors.New("project members are not synced; run `panda pull` first")
	}
	return members, nil
}

func projectMembersIfAvailable(ws *workspace.Workspace) ([]map[string]any, bool, error) {
	data, err := os.ReadFile(filepath.Join(ws.Root, ".panda", "members.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var members []map[string]any
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, false, fmt.Errorf("parse project members: %w", err)
	}
	return members, true, nil
}

func memberUser(member map[string]any) map[string]any {
	user, _ := member["user"].(map[string]any)
	return user
}

func memberDisplayName(member map[string]any) string {
	user := memberUser(member)
	for _, field := range []string{"label", "name", "email", "id"} {
		if value := strings.TrimSpace(stringField(user, field)); value != "" {
			return value
		}
	}
	return "(unnamed member)"
}

func editAssignees(ws *workspace.Workspace, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	if len(requested) == 1 && isNoneSelector(requested[0]) {
		metadata["assignees"] = []any{}
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	allMembers, err := projectMembers(ws)
	if err != nil {
		return err
	}
	members := eligibleProjectMembers(allMembers)
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(memberDisplayName(members[i])) < strings.ToLower(memberDisplayName(members[j]))
	})
	byID := make(map[string]map[string]any, len(members))
	for _, member := range members {
		user := memberUser(member)
		if id := relationshipID(user); id != "" {
			byID[id] = user
		}
	}
	selected := make(map[string]map[string]any)
	if current, ok := metadata["assignees"].([]any); ok {
		for _, item := range current {
			if relation, ok := item.(map[string]any); ok {
				if id := relationshipID(relation); id != "" && byID[id] != nil {
					selected[id] = cloneMap(relation)
				}
			}
		}
	}
	if len(requested) > 0 {
		selected = make(map[string]map[string]any)
		for _, name := range requested {
			name = strings.TrimSpace(name)
			match, err := resolveAssignableProjectMember(members, allMembers, name)
			if err != nil {
				return err
			}
			user := memberUser(match)
			selected[relationshipID(user)] = map[string]any{"id": relationshipID(user), "label": memberDisplayName(match)}
		}
	} else if isInteractiveTerminal(in, out) {
		choices := make([]selectorOption, 0, len(members))
		for _, member := range members {
			id := relationshipID(memberUser(member))
			choices = append(choices, selectorOption{value: id, label: memberDisplayName(member) + " (" + stringField(member, "role") + ")"})
		}
		selectedIDs := make(map[string]bool, len(selected))
		for id := range selected {
			selectedIDs[id] = true
		}
		selectedIDs, err = selectMultipleOptions(in, out, "Assignees", choices, selectedIDs)
		if err != nil {
			return err
		}
		selected = make(map[string]map[string]any, len(selectedIDs))
		for id, isSelected := range selectedIDs {
			if !isSelected {
				continue
			}
			if user := byID[id]; user != nil {
				selected[id] = map[string]any{"id": id, "label": memberDisplayName(map[string]any{"user": user})}
			}
		}
	} else {
		fmt.Fprintln(out, "Project team members:")
		for index, member := range members {
			fmt.Fprintf(out, "  %d) %s (%s)\n", index+1, memberDisplayName(member), stringField(member, "role"))
		}
		choice, err := prompt(in, out, "Assignees (comma-separated names, `none` to clear, Enter to keep): ")
		if err != nil {
			return err
		}
		if choice != "" {
			if isNoneSelector(choice) {
				selected = make(map[string]map[string]any)
			} else {
				selected = make(map[string]map[string]any)
				for _, name := range strings.Split(choice, ",") {
					match, err := resolveAssignableProjectMember(members, allMembers, strings.TrimSpace(name))
					if err != nil {
						return err
					}
					user := memberUser(match)
					selected[relationshipID(user)] = map[string]any{"id": relationshipID(user), "label": memberDisplayName(match)}
				}
			}
		}
	}
	selectedIDs := make(map[string]bool, len(selected))
	for id := range selected {
		selectedIDs[id] = true
	}
	_, _, err = saveAssigneeSelection(dir, metadata, selectedIDs, selected)
	return err
}

func assignableRole(role string) (string, bool) {
	for _, candidate := range assignableTeamRoles {
		if strings.EqualFold(candidate, role) {
			return candidate, true
		}
	}
	return "", false
}

func eligibleProjectMembers(members []map[string]any) []map[string]any {
	eligible := make([]map[string]any, 0, len(members))
	for _, member := range members {
		if _, ok := assignableRole(stringField(member, "role")); ok {
			eligible = append(eligible, member)
		}
	}
	return eligible
}

func assignTaskByPerson(ws *workspace.Workspace, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: panda assign <taskNumber|draft-N> <person[,person...]|none>")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	selectors := make([]string, 0, len(args)-1)
	for _, arg := range args[1:] {
		for _, value := range strings.Split(arg, ",") {
			value = strings.TrimSpace(value)
			if value != "" {
				selectors = append(selectors, value)
			}
		}
	}
	if len(selectors) == 0 {
		return errors.New("panda assign requires one or more people, or none to clear assignees")
	}
	selected := make(map[string]bool)
	relations := make(map[string]map[string]any)
	clear := len(selectors) == 1 && isNoneSelector(selectors[0])
	for _, selector := range selectors {
		if isNoneSelector(selector) && !clear {
			return errors.New("none must be used alone to clear assignees")
		}
	}
	if !clear {
		allMembers, err := projectMembers(ws)
		if err != nil {
			return err
		}
		members := eligibleProjectMembers(allMembers)
		for _, selector := range selectors {
			match, err := resolveAssignableProjectMember(members, allMembers, selector)
			if err != nil {
				return err
			}
			user := memberUser(match)
			id := relationshipID(user)
			selected[id] = true
			relations[id] = map[string]any{"id": id, "label": memberDisplayName(match)}
		}
	}
	updated, changed, err := saveAssigneeSelection(dir, metadata, selected, relations)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintf(out, "Cleared assignees for task %s.\n", taskLabel(updated))
	} else if changed {
		fmt.Fprintf(out, "Assigned %d person(s) to task %s: %s.\n", len(selected), taskLabel(updated), displayedAssignees(updated))
	} else {
		fmt.Fprintf(out, "No assignment changes for task %s.\n", taskLabel(updated))
	}
	return nil
}

func findCatalogRecordByName(records []map[string]any, kind catalogKind, name string) map[string]any {
	for _, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), strings.TrimSpace(name)) || catalogRecordID(record) == name {
			return record
		}
	}
	return nil
}

func isNoneSelector(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "none") || strings.TrimSpace(value) == "-"
}

func hasPackageAssignment(value any) bool {
	if relation, ok := value.(map[string]any); ok {
		return relationshipID(relation) != "" || relationshipDisplayName(relation) != ""
	}
	return !isEmptyField(value)
}

func hasLabelAssignments(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}

func relationshipDisplayName(value any) string {
	relation, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, field := range []string{"label", "name", "title", "id"} {
		if name := strings.TrimSpace(stringField(relation, field)); name != "" {
			return name
		}
	}
	return ""
}

func catalogNameIndex(kind catalogKind, records []map[string]any) map[string]string {
	index := make(map[string]string, len(records))
	for _, record := range records {
		id := catalogRecordID(record)
		if id != "" {
			index[id] = catalogRecordName(kind, record)
		}
	}
	return index
}

func relationshipDisplayNameWithCatalog(value any, catalogNames map[string]string) string {
	if id := relationshipID(value); id != "" {
		if name := catalogNames[id]; name != "" {
			return name
		}
	}
	return relationshipDisplayName(value)
}

func labelsDisplayWithCatalog(value any, catalogNames map[string]string) string {
	items, _ := value.([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := relationshipDisplayNameWithCatalog(item, catalogNames); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return strings.Join(names, ", ")
}

func assigneesDisplay(value any) string {
	items, _ := value.([]any)
	var names []string
	for _, item := range items {
		if name := relationshipDisplayName(item); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return strings.Join(names, ", ")
}

func remapTaskCatalogRelationships(task map[string]any, idMap map[string]string) (map[string]any, error) {
	result := cloneMap(task)
	if labels, ok := result["labels"].([]any); ok {
		mapped := make([]any, 0, len(labels))
		for _, value := range labels {
			relation, ok := value.(map[string]any)
			if !ok {
				mapped = append(mapped, value)
				continue
			}
			updated, err := mapCatalogRelation(relation, idMap)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, updated)
		}
		result["labels"] = mapped
	}
	if relation, ok := result["package"].(map[string]any); ok {
		updated, err := mapCatalogRelation(relation, idMap)
		if err != nil {
			return nil, err
		}
		result["package"] = updated
	}
	return result, nil
}

func rewriteTaskCatalogIDs(ws *workspace.Workspace, idMap map[string]string) error {
	if len(idMap) == 0 {
		return nil
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		path := filepath.Join(dir, "metadata.json")
		metadata, err := workspace.ReadMap(path)
		if err != nil {
			return err
		}
		updated := cloneMap(metadata)
		changed := false
		if labels, ok := updated["labels"].([]any); ok {
			mapped := make([]any, 0, len(labels))
			for _, value := range labels {
				relation, ok := value.(map[string]any)
				if ok {
					if remoteID, exists := idMap[relationshipID(relation)]; exists {
						relation = cloneMap(relation)
						relation["id"] = remoteID
						changed = true
					}
					mapped = append(mapped, relation)
				} else {
					mapped = append(mapped, value)
				}
			}
			updated["labels"] = mapped
		}
		if relation, ok := updated["package"].(map[string]any); ok {
			if remoteID, exists := idMap[relationshipID(relation)]; exists {
				relation = cloneMap(relation)
				relation["id"] = remoteID
				updated["package"] = relation
				changed = true
			}
		}
		if changed {
			if err := workspace.WriteMap(path, updated); err != nil {
				return err
			}
		}
	}
	return nil
}

func mapCatalogRelation(relation map[string]any, idMap map[string]string) (map[string]any, error) {
	updated := cloneMap(relation)
	id := relationshipID(updated)
	if remoteID, exists := idMap[id]; exists {
		updated["id"] = remoteID
		return updated, nil
	}
	if strings.HasPrefix(id, "local-label-") || strings.HasPrefix(id, "local-package-") {
		return nil, fmt.Errorf("catalog item %q has not been pushed yet", relationshipDisplayName(updated))
	}
	return updated, nil
}
