package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func editLabels(ws *workspace.Workspace, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	labels, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return err
	}
	sort.Slice(labels, func(i, j int) bool {
		return strings.ToLower(catalogRecordName(labelCatalog, labels[i])) < strings.ToLower(catalogRecordName(labelCatalog, labels[j]))
	})
	if len(labels) == 0 {
		fmt.Fprintln(out, "No project labels. Create one with `panda labels create` or run `panda pull`.")
	}
	selected := make(map[string]map[string]any)
	if values, ok := metadata["labels"].([]any); ok {
		for _, value := range values {
			if relation, ok := value.(map[string]any); ok {
				if id := relationshipID(relation); id != "" {
					selected[id] = map[string]any{"id": id, "label": relationshipDisplayName(relation)}
				}
			}
		}
	}
	if len(requested) > 0 {
		if len(requested) == 1 && isNoneSelector(requested[0]) {
			selected = make(map[string]map[string]any)
		} else {
			for _, name := range requested {
				remove := strings.HasPrefix(name, "--remove=")
				name = strings.TrimPrefix(name, "--remove=")
				match := findCatalogRecordByName(labels, labelCatalog, name)
				if match == nil {
					return fmt.Errorf("label %q not found in the local project catalog; create it with `panda labels create` or run `panda pull`", name)
				}
				id := catalogRecordID(match)
				if remove {
					delete(selected, id)
				} else {
					selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, match)}
				}
			}
		}
	} else if isInteractiveTerminal(in, out) {
		pickerLabels := cloneCatalogRecords(labels)
		known := catalogNameIndex(labelCatalog, labels)
		for id, relation := range selected {
			if known[id] == "" {
				pickerLabels = append(pickerLabels, map[string]any{"id": id, "name": relationshipDisplayName(relation)})
			}
		}
		sort.Slice(pickerLabels, func(i, j int) bool {
			return strings.ToLower(catalogRecordName(labelCatalog, pickerLabels[i])) < strings.ToLower(catalogRecordName(labelCatalog, pickerLabels[j]))
		})
		choices := make([]selectorOption, 0, len(pickerLabels))
		for _, label := range pickerLabels {
			id := catalogRecordID(label)
			choices = append(choices, selectorOption{value: id, label: catalogRecordName(labelCatalog, label)})
		}
		selectedIDs := make(map[string]bool, len(selected))
		for id := range selected {
			selectedIDs[id] = true
		}
		selectedIDs, err = selectMultipleOptions(in, out, "Labels", choices, selectedIDs)
		if err != nil {
			return err
		}
		selected = make(map[string]map[string]any, len(selectedIDs))
		for _, label := range pickerLabels {
			id := catalogRecordID(label)
			if selectedIDs[id] {
				selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, label)}
			}
		}
	} else {
		finished := false
		for !finished {
			prefix, err := prompt(in, out, fmt.Sprintf("Search %s by prefix (blank to finish, `none` to clear all): ", colorText(out, "1", "labels")))
			if err != nil {
				return err
			}
			if strings.EqualFold(prefix, "none") {
				selected = make(map[string]map[string]any)
				break
			}
			if prefix == "" {
				break
			}
			for {
				matches := make([]map[string]any, 0)
				for _, label := range labels {
					if strings.HasPrefix(strings.ToLower(catalogRecordName(labelCatalog, label)), strings.ToLower(prefix)) {
						matches = append(matches, label)
					}
				}
				if len(matches) == 0 {
					fmt.Fprintln(out, "No labels match that prefix.")
					break
				}
				for index, label := range matches {
					id := catalogRecordID(label)
					mark := " "
					if _, exists := selected[id]; exists {
						mark = "✓"
					}
					fmt.Fprintf(out, "  %s %d) %s\n", mark, index+1, catalogRecordName(labelCatalog, label))
				}
				choice, err := prompt(in, out, "Select a number to toggle, type a narrower prefix, or press Enter to finish: ")
				if err != nil {
					return err
				}
				if choice == "" {
					finished = true
					break
				}
				index, parseErr := strconv.Atoi(choice)
				if parseErr != nil || index < 1 || index > len(matches) {
					prefix = choice
					continue
				}
				label := matches[index-1]
				id := catalogRecordID(label)
				if _, exists := selected[id]; exists {
					delete(selected, id)
				} else {
					selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, label)}
				}
				break
			}
		}
	}
	result := make([]any, 0, len(selected))
	for _, relation := range selected {
		result = append(result, relation)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := result[i].(map[string]any)
		right, _ := result[j].(map[string]any)
		return stringField(left, "label") < stringField(right, "label")
	})
	metadata["labels"] = result
	return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
}

func editTaskPackage(ws *workspace.Workspace, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	if stringField(metadata, "type") == "release" {
		if len(requested) == 1 && isNoneSelector(requested[0]) {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		return errors.New("release tasks cannot be assigned to a package")
	}
	packages, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return err
	}
	sort.Slice(packages, func(i, j int) bool {
		return strings.ToLower(catalogRecordName(packageCatalog, packages[i])) < strings.ToLower(catalogRecordName(packageCatalog, packages[j]))
	})
	if len(requested) > 0 {
		selection := strings.TrimSpace(strings.Join(requested, " "))
		if isNoneSelector(selection) {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		match := findCatalogRecordByName(packages, packageCatalog, selection)
		if match == nil {
			return fmt.Errorf("package %q not found in the local project catalog; create it with `panda packages create` or run `panda pull`", selection)
		}
		metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	if isInteractiveTerminal(in, out) {
		choices := make([]selectorOption, 0, len(packages)+1)
		choices = append(choices, selectorOption{value: "", label: "None (no package)"})
		currentID := relationshipID(metadata["package"])
		currentInCatalog := false
		for _, pkg := range packages {
			id := catalogRecordID(pkg)
			choices = append(choices, selectorOption{value: id, label: catalogRecordName(packageCatalog, pkg)})
			currentInCatalog = currentInCatalog || id == currentID
		}
		if currentID != "" && !currentInCatalog {
			choices = append(choices, selectorOption{value: currentID, label: relationshipDisplayName(metadata["package"]) + " (current)"})
		}
		selected, err := selectSingleOption(in, out, "Package", choices, currentID)
		if err != nil {
			return err
		}
		if selected == "" {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		match := findCatalogRecordByName(packages, packageCatalog, selected)
		if match == nil {
			if selected == currentID {
				return nil
			}
			return fmt.Errorf("package %q is not in the local project catalog", selected)
		}
		metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	current := relationshipDisplayName(metadata["package"])
	if len(packages) == 0 {
		fmt.Fprintln(out, "No project packages. Create one with `panda packages create` or run `panda pull`.")
		return nil
	}
	fmt.Fprintln(out, "Available packages:")
	for index, pkg := range packages {
		fmt.Fprintf(out, "  %d) %s\n", index+1, catalogRecordName(packageCatalog, pkg))
	}
	choice, err := prompt(in, out, fmt.Sprintf("%s [%s] (number or name, `none` to clear, Enter to keep): ", colorText(out, "1", "Package"), current))
	if err != nil {
		return err
	}
	if choice == "" {
		return nil
	}
	if isNoneSelector(choice) {
		delete(metadata, "package")
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	var match map[string]any
	if index, parseErr := strconv.Atoi(choice); parseErr == nil && index >= 1 && index <= len(packages) {
		match = packages[index-1]
	} else {
		match = findCatalogRecordByName(packages, packageCatalog, choice)
	}
	if match == nil {
		return fmt.Errorf("package %q not found in the local project catalog", choice)
	}
	metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
	return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
}
