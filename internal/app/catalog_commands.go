package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func projectCatalogCommand(ws *workspace.Workspace, project *config.Project, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
		return listCatalog(ws, kind, out)
	}
	switch args[0] {
	case "create":
		return createCatalogRecordLocal(ws, project, kind, args[1:], in, out)
	case "edit":
		return editCatalogRecordLocal(ws, kind, args[1:], in, out)
	case "delete", "remove":
		return deleteCatalogRecordLocal(ws, kind, args[1:], out)
	default:
		return fmt.Errorf("usage: panda %s [list | create [name|title] [description] | edit <name|title> | delete <name|title>]", kind)
	}
}

func listCatalog(ws *workspace.Workspace, kind catalogKind, out io.Writer) error {
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	baseline, err := readCatalogBaseline(ws, kind)
	if err != nil {
		return err
	}
	baseByID := make(map[string]map[string]any, len(baseline))
	for _, record := range baseline {
		baseByID[stringField(record, "id")] = record
	}
	if kind == labelCatalog {
		fmt.Fprintln(out, "Project labels")
	} else {
		fmt.Fprintln(out, "Project packages")
	}
	for _, record := range records {
		status := ""
		if stringField(record, "localId") != "" {
			status = " [new]"
		} else if base := baseByID[stringField(record, "id")]; base != nil && catalogRecordChanged(kind, record, base) {
			status = " [edited]"
		}
		name := catalogRecordName(kind, record)
		if description := strings.TrimSpace(stringField(record, "description")); description != "" {
			fmt.Fprintf(out, "  %s%s — %s\n", name, status, description)
		} else {
			fmt.Fprintf(out, "  %s%s\n", name, status)
		}
	}
	deleted := 0
	localIDs := make(map[string]bool)
	for _, record := range records {
		localIDs[stringField(record, "id")] = true
	}
	for _, record := range baseline {
		if !localIDs[stringField(record, "id")] {
			deleted++
		}
	}
	if deleted > 0 {
		fmt.Fprintf(out, "  %d deletion(s) staged\n", deleted)
	}
	if len(records) == 0 && deleted == 0 {
		fmt.Fprintf(out, "  No project %s.\n", kind)
	}
	return nil
}

func createCatalogRecordLocal(ws *workspace.Workspace, project *config.Project, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	name := ""
	description := ""
	supplemental := make(map[string]any)
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
		description = strings.TrimSpace(strings.Join(args[1:], " "))
	} else {
		var err error
		nameFieldTitle := catalogNameField(kind)
		name, err = prompt(in, out, fmt.Sprintf("%s: ", colorText(out, "1", strings.ToUpper(nameFieldTitle[:1])+nameFieldTitle[1:])))
		if err != nil {
			return err
		}
		description, err = prompt(in, out, fmt.Sprintf("%s (optional): ", colorText(out, "1", "Description")))
		if err != nil {
			return err
		}
		if kind == packageCatalog {
			if err := editPackageSupplementalFields(supplemental, in, out); err != nil {
				return err
			}
		}
	}
	if name == "" {
		return fmt.Errorf("%s %s cannot be empty", catalogItemName(kind), catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	for _, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), name) {
			return fmt.Errorf("%s %q already exists in this project", catalogItemName(kind), name)
		}
	}
	localID := fmt.Sprintf("local-%s-%s", strings.TrimSuffix(string(kind), "s"), strconv.FormatInt(time.Now().UnixNano(), 36))
	record := map[string]any{
		"localId": localID, "project": map[string]any{"id": project.ID, "label": project.Name},
		catalogNameField(kind): name, "description": description,
	}
	for field, value := range supplemental {
		record[field] = value
	}
	records = append(records, record)
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), name)
	return nil
}

func editCatalogRecordLocal(ws *workspace.Workspace, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: panda %s edit <%s>", kind, catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	index := findCatalogRecord(records, kind, args[0])
	if index < 0 {
		return fmt.Errorf("%s %q was not found in the local project catalog; run `panda pull` first", catalogItemName(kind), args[0])
	}
	record := records[index]
	nameField := catalogNameField(kind)
	nameTitle := strings.ToUpper(nameField[:1]) + nameField[1:]
	name, err := prompt(in, out, fmt.Sprintf("%s [%s]: ", colorText(out, "1", nameTitle), catalogRecordName(kind, record)))
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) != "" {
		record[nameField] = strings.TrimSpace(name)
	}
	description, err := prompt(in, out, fmt.Sprintf("%s [Enter keeps current, - clears]: ", colorText(out, "1", "Description")))
	if err != nil {
		return err
	}
	if description == "-" {
		record["description"] = ""
	} else if strings.TrimSpace(description) != "" {
		record["description"] = description
	}
	if kind == packageCatalog {
		if err := editPackageSupplementalFields(record, in, out); err != nil {
			return err
		}
	}
	if err := ensureCatalogNameUnique(records, kind, index, catalogRecordName(kind, record)); err != nil {
		return err
	}
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), catalogRecordName(kind, record))
	return nil
}

func editPackageSupplementalFields(record map[string]any, in io.Reader, out io.Writer) error {
	startDate, err := prompt(in, out, fmt.Sprintf("%s [%s] (YYYY-MM-DD, - clears): ", colorText(out, "1", "Start date"), stringField(record, "startDate")))
	if err != nil {
		return err
	}
	if startDate == "-" {
		record["startDate"] = ""
	} else if strings.TrimSpace(startDate) != "" {
		record["startDate"] = strings.TrimSpace(startDate)
	}
	currentEstimation := ""
	if value := record["estimation"]; value != nil {
		currentEstimation = fmt.Sprint(value)
	}
	estimation, err := prompt(in, out, fmt.Sprintf("%s [%s] (- clears): ", colorText(out, "1", "Estimation"), currentEstimation))
	if err != nil {
		return err
	}
	if estimation == "-" {
		record["estimation"] = nil
	} else if strings.TrimSpace(estimation) != "" {
		value, err := strconv.ParseFloat(strings.TrimSpace(estimation), 64)
		if err != nil || value < 0 {
			return fmt.Errorf("invalid package estimation %q; enter a nonnegative number", estimation)
		}
		record["estimation"] = value
	}
	return nil
}

func deleteCatalogRecordLocal(ws *workspace.Workspace, kind catalogKind, args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: panda %s delete <%s>", kind, catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	index := findCatalogRecord(records, kind, args[0])
	if index < 0 {
		return fmt.Errorf("%s %q was not found in the local project catalog", catalogItemName(kind), args[0])
	}
	deleted := records[index]
	identifier := catalogRecordID(deleted)
	records = append(records[:index], records[index+1:]...)
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	if err := removeCatalogAssignments(ws, kind, identifier); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), catalogRecordName(kind, deleted))
	return nil
}

func findCatalogRecord(records []map[string]any, kind catalogKind, selector string) int {
	for index, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), selector) || catalogRecordID(record) == selector {
			return index
		}
	}
	return -1
}

func ensureCatalogNameUnique(records []map[string]any, kind catalogKind, ignore int, name string) error {
	for index, record := range records {
		if index != ignore && strings.EqualFold(catalogRecordName(kind, record), name) {
			return fmt.Errorf("%s %q already exists in this project", catalogItemName(kind), name)
		}
	}
	return nil
}

func removeCatalogAssignments(ws *workspace.Workspace, kind catalogKind, identifier string) error {
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
		changed := false
		if kind == labelCatalog {
			labels, _ := metadata["labels"].([]any)
			filtered := make([]any, 0, len(labels))
			for _, value := range labels {
				relation, _ := value.(map[string]any)
				if relationshipID(relation) == identifier {
					changed = true
					continue
				}
				filtered = append(filtered, value)
			}
			if changed {
				metadata["labels"] = filtered
			}
		} else if relation, ok := metadata["package"].(map[string]any); ok && relationshipID(relation) == identifier {
			delete(metadata, "package")
			changed = true
		}
		if changed {
			if err := workspace.WriteMap(path, metadata); err != nil {
				return err
			}
		}
	}
	return nil
}
