package app

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func viewTask(ws *workspace.Workspace, args []string, out io.Writer) error {
	return writePagedOutput(out, func(rendered io.Writer) error {
		return renderTaskView(ws, args, rendered, out)
	})
}

func renderTaskView(ws *workspace.Workspace, args []string, out, colorTarget io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: panda view <taskNumber|draft-N>")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	labelsCatalog, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return err
	}
	packagesCatalog, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return err
	}
	labelNames := catalogNameIndex(labelCatalog, labelsCatalog)
	packageNames := catalogNameIndex(packageCatalog, packagesCatalog)
	title := taskLabel(metadata) + " — " + stringField(metadata, "title")
	fmt.Fprintf(out, "%s\n\n", colorText(colorTarget, "1;32", "# "+title))
	for _, field := range []string{"type", "status", "priority", "complexity", "package", "labels", "assignees", "reviewers"} {
		if value, exists := metadata[field]; exists {
			if field == "package" {
				value = relationshipDisplayNameWithCatalog(value, packageNames)
			} else if field == "labels" {
				value = labelsDisplayWithCatalog(value, labelNames)
			} else if field == "assignees" {
				value = assigneesDisplay(value)
			}
			fmt.Fprintf(out, "%s %v  \n", colorText(colorTarget, "1;36", "**"+field+":**"), value)
		}
	}
	fmt.Fprintln(out, colorText(colorTarget, "1;32", "\n## Description"))
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return err
	}
	fmt.Fprintln(out, highlightMarkdown(out, string(description)))
	fmt.Fprintln(out, colorText(colorTarget, "1;32", "\n## Notes"))
	notesDir := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	state, _ := ws.ReadState(stateKey(dir, metadata))
	staged := make(map[string]bool, len(state.StagedNotes))
	for _, name := range state.StagedNotes {
		staged[name] = true
	}
	printed := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(notesDir, entry.Name()))
		if err != nil {
			return err
		}
		marker := ""
		if staged[entry.Name()] {
			marker = colorText(colorTarget, "1;33", " [staged]")
		}
		fmt.Fprintf(out, "%s%s\n", colorText(colorTarget, "1;36", "### "+strings.TrimSuffix(entry.Name(), ".md")), marker)
		fmt.Fprintln(out, highlightMarkdown(out, string(body)))
		printed = true
	}
	if !printed {
		fmt.Fprintln(out, "No notes.")
	}
	return nil
}

func linkTask(ws *workspace.Workspace, args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: panda link <taskNumber|draft-N>")
	}
	_, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	taskID := stringField(metadata, "id")
	if taskID == "" {
		return fmt.Errorf("task %s has no verified Solutions task ID; publish it with `panda push` first", taskLabel(metadata))
	}
	state, err := ws.ReadState(taskID)
	if err != nil || state.LocalCreated || state.Remote == nil || stringField(state.Remote, "id") != taskID {
		return fmt.Errorf("task %s has no verified remote sync baseline; run `panda pull` first", taskLabel(metadata))
	}
	if !same(valueAt(metadata, "number"), valueAt(state.Remote, "number")) {
		return fmt.Errorf("task %s number does not match its verified remote baseline", taskLabel(metadata))
	}
	link, err := canonicalTaskLink(metadata["number"], taskID)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, link)
	return nil
}

func canonicalTaskLink(taskNumber any, taskID string) (string, error) {
	numberText := ""
	if taskNumber != nil {
		numberText = strings.TrimSpace(fmt.Sprint(taskNumber))
	}
	if numberText == "" {
		return "", errors.New("task number is missing")
	}
	number, err := strconv.ParseFloat(numberText, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return "", fmt.Errorf("task number %q is invalid", numberText)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", errors.New("verified task ID is missing")
	}
	return fmt.Sprintf("[#%s](https://solutions.slingrs.io/prod/runtime/#views/%s/readOnly/%s)", numberText, url.PathEscape(solutions.TaskViewId), url.PathEscape(taskID)), nil
}
