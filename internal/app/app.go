package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/jsonutil"
	"panda-dashboard/internal/task"
	"panda-dashboard/internal/ui"
	"panda-dashboard/internal/workspace"
)

var editableFields = task.EditableFields

func projectWorkspace(cfg *config.Config) (*workspace.Workspace, error) {
	if cfg.Project == nil || cfg.Project.ID == "" || cfg.Project.Name == "" {
		return nil, errors.New("no project selected; run `panda config set project <name>` first")
	}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		return nil, err
	}
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		return nil, err
	}
	return ws, nil
}

func requireLogin(cfg *config.Config) error {
	if cfg.Token == "" {
		return errors.New("not logged in; run `panda login` first")
	}
	return nil
}

func createTask(ws *workspace.Workspace, project *config.Project, args []string, in io.Reader, out io.Writer) error {
	options, positional, err := parseTaskFieldOptions(args)
	if err != nil {
		return err
	}
	if options.supplied {
		return createTaskWithOptions(ws, project.ID, project.Name, options, positional, in, out)
	}
	return createTaskInteractive(ws, project, args, in, out)
}

func createTaskInteractive(ws *workspace.Workspace, project *config.Project, args []string, in io.Reader, out io.Writer) error {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		var err error
		title, err = prompt(in, out, fmt.Sprintf("%s: ", colorText(out, "1", "Title")))
		if err != nil {
			return err
		}
	}
	if title == "" {
		return errors.New("task title cannot be empty")
	}
	localID := "local-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dir := ws.TaskPath(localID)
	metadata := map[string]any{
		"localId":   localID,
		"project":   map[string]any{"id": project.ID, "label": project.Name},
		"title":     title,
		"type":      "feature",
		"priority":  "medium",
		"status":    "new",
		"labels":    []any{},
		"assignees": []any{},
		"reviewers": []any{},
	}
	if isInteractiveTerminal(in, out) {
		if err := editMetadataFields(metadata, false, in, out); err != nil {
			return err
		}
		if err := editLabels(ws, dir, metadata, nil, in, out); err != nil {
			return err
		}
		if err := editTaskPackage(ws, dir, metadata, nil, in, out); err != nil {
			return err
		}
		if err := editAssignees(ws, dir, metadata, nil, in, out); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		return err
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), nil, 0o600); err != nil {
		return err
	}
	if isInteractiveTerminal(in, out) {
		if err := promptDescriptionEditor(in, out, dir); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
	}
	if err := ws.WriteState(localID, workspace.State{LocalCreated: true}); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created local task %s. Run `panda edit %s` to edit it and `panda push` to publish it.\n", localID, localID)
	return nil
}

func editTask(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: panda edit <taskNumber|localId> [labels [labelName...]|package [name|none]|assignees [member...|none]|json]")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		switch args[1] {
		case "labels":
			return editLabels(ws, dir, metadata, args[2:], in, out)
		case "package":
			return editTaskPackage(ws, dir, metadata, args[2:], in, out)
		case "assignees", "assign":
			return editAssignees(ws, dir, metadata, args[2:], in, out)
		case "json":
			if len(args) != 2 {
				return errors.New("usage: panda edit <taskNumber|localId> json")
			}
			if err := openEditor(filepath.Join(dir, "metadata.json")); err != nil {
				return err
			}
			return promptDescriptionEditor(in, out, dir)
		default:
			return errors.New("field selectors are `labels`, `package`, `assignees`, and `json`")
		}
	}
	options, positional, err := parseTaskFieldOptions(args[1:])
	if err != nil {
		return err
	}
	if options.supplied {
		if len(positional) > 0 {
			return fmt.Errorf("unexpected argument %q; use task field options", positional[0])
		}
		updated, description, err := applyTaskFieldOptions(ws, metadata, options, in)
		if err != nil {
			return err
		}
		if options.descriptionProvided {
			if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), description, 0o600); err != nil {
				return fmt.Errorf("save task description: %w", err)
			}
		}
		if options.hasMetadataFields() {
			if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), updated); err != nil {
				return fmt.Errorf("save task metadata: %w", err)
			}
		}
		return nil
	}
	if err := editMetadataInteractive(dir, metadata, in, out); err != nil {
		return err
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return err
	}
	if stringField(metadata, "type") == "release" && hasPackageAssignment(metadata["package"]) {
		delete(metadata, "package")
		if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
			return err
		}
	}
	if isInteractiveTerminal(in, out) {
		metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return err
		}
		if err := editLabels(ws, dir, metadata, nil, in, out); err != nil {
			return err
		}
		if stringField(metadata, "type") != "release" {
			metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
			if err != nil {
				return err
			}
			if err := editTaskPackage(ws, dir, metadata, nil, in, out); err != nil {
				return err
			}
		}
		metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return err
		}
		if err := editAssignees(ws, dir, metadata, nil, in, out); err != nil {
			return err
		}
	} else {
		editLabelsChoice, err := prompt(in, out, "Edit labels from the local project catalog? [y/N] ")
		if err != nil {
			return err
		}
		if strings.EqualFold(editLabelsChoice, "y") || strings.EqualFold(editLabelsChoice, "yes") {
			metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
			if err != nil {
				return err
			}
			if err := editLabels(ws, dir, metadata, nil, in, out); err != nil {
				return err
			}
		}
		if stringField(metadata, "type") != "release" {
			packageChoice, err := prompt(in, out, "Edit package? [y/N] ")
			if err != nil {
				return err
			}
			if strings.EqualFold(packageChoice, "y") || strings.EqualFold(packageChoice, "yes") {
				metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
				if err != nil {
					return err
				}
				if err := editTaskPackage(ws, dir, metadata, nil, in, out); err != nil {
					return err
				}
			}
		}
		assigneeChoice, err := prompt(in, out, "Edit assignees? [y/N] ")
		if err != nil {
			return err
		}
		if strings.EqualFold(assigneeChoice, "y") || strings.EqualFold(assigneeChoice, "yes") {
			metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
			if err != nil {
				return err
			}
			if err := editAssignees(ws, dir, metadata, nil, in, out); err != nil {
				return err
			}
		}
	}
	return promptDescriptionEditor(in, out, dir)
}

func promptDescriptionEditor(in io.Reader, out io.Writer, dir string) error {
	if isInteractiveTerminal(in, out) {
		key, err := readSingleKey(in, out)
		if err != nil {
			return err
		}
		if descriptionEditorRequested(key) {
			return openEditor(filepath.Join(dir, "description.md"))
		}
		return nil
	}
	choice, err := prompt(in, out, fmt.Sprintf("%s [(e) to launch editor, enter to skip]: ", colorText(out, "1", "Description")))
	if err != nil {
		return err
	}
	if strings.EqualFold(choice, "e") {
		return openEditor(filepath.Join(dir, "description.md"))
	}
	return nil
}

func editMetadataInteractive(dir string, metadata map[string]any, in io.Reader, out io.Writer) error {
	if err := editMetadataFields(metadata, true, in, out); err != nil {
		return err
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	return nil
}

func editMetadataFields(metadata map[string]any, includeTitle bool, in io.Reader, out io.Writer) error {
	if includeTitle {
		title, err := prompt(in, out, fmt.Sprintf("%s [%s]: ", colorText(out, "1", "Title"), stringField(metadata, "title")))
		if err != nil {
			return err
		}
		if title != "" {
			metadata["title"] = title
		}
	}
	for _, option := range []struct {
		field  string
		values []string
	}{
		{field: "type", values: []string{"feature", "bug", "other", "release"}},
		{field: "priority", values: []string{"hotfix", "high", "medium", "low"}},
		{field: "complexity", values: []string{"xs", "s", "m", "l", "xl", "xxl"}},
	} {
		current := stringField(metadata, option.field)
		fieldTitle := strings.ToUpper(option.field[:1]) + option.field[1:]
		if isInteractiveTerminal(in, out) {
			choices := make([]selectorOption, 0, len(option.values)+1)
			if option.field == "complexity" {
				choices = append(choices, selectorOption{value: "", label: "None"})
			}
			for _, value := range option.values {
				choices = append(choices, selectorOption{value: value, label: value})
			}
			selected, err := selectSingleOption(in, out, fieldTitle, choices, current)
			if err != nil {
				return err
			}
			if option.field == "complexity" && selected == "" {
				delete(metadata, option.field)
			} else {
				metadata[option.field] = selected
			}
			continue
		}
		choice, err := prompt(in, out, fmt.Sprintf("%s [%s] (%s): ", colorText(out, "1", fieldTitle), current, strings.Join(option.values, "/")))
		if err != nil {
			return err
		}
		if choice == "-" && option.field == "complexity" {
			delete(metadata, option.field)
			continue
		}
		if choice == "" {
			continue
		}
		if !contains(option.values, choice) {
			return fmt.Errorf("invalid %s %q; choose one of %s", option.field, choice, strings.Join(option.values, ", "))
		}
		metadata[option.field] = choice
	}
	return nil
}

func prompt(in io.Reader, out io.Writer, message string) (string, error) {
	return ui.Prompt(in, out, message)
}

func openEditor(path string) error {
	return ui.OpenEditor(path)
}

func editTemporaryNote(in io.Reader) (string, error) {
	return ui.EditTemporaryNote(in)
}

func stateKey(dir string, metadata map[string]any) string {
	if id := stringField(metadata, "id"); id != "" {
		return id
	}
	return filepath.Base(dir)
}

func cloneMap(input map[string]any) map[string]any {
	return jsonutil.CloneMap(input)
}

func valueAt(values map[string]any, key string) any {
	return jsonutil.ValueAt(values, key)
}

func same(left, right any) bool {
	return jsonutil.Same(left, right)
}

func stringField(values map[string]any, key string) string {
	return jsonutil.StringField(values, key)
}

func relationshipID(value any) string {
	return jsonutil.RelationshipID(value)
}

func taskLabel(metadata map[string]any) string {
	if number := metadata["number"]; number != nil {
		return fmt.Sprint(number)
	}
	if id := stringField(metadata, "id"); id != "" {
		return id
	}
	if localID := stringField(metadata, "localId"); localID != "" {
		return localID
	}
	return "local task"
}

func isConflictField(fields []string, field string) bool {
	for _, conflict := range fields {
		if conflict == field {
			return true
		}
	}
	return false
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func safeName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else if r == ' ' {
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func fprintln(out io.Writer) {
	fmt.Fprintln(out)
}

func noteFilename(timestamp time.Time, author string, duplicateIndex int) string {
	suffix := ""
	if duplicateIndex > 1 {
		suffix = fmt.Sprintf("-%d", duplicateIndex)
	}
	return timestamp.UTC().Format("20060102T150405.000Z") + "-" + safeName(author) + suffix + ".md"
}

func uniqueNoteFilename(notesDir string, timestamp time.Time, author string) (string, error) {
	for index := 1; ; index++ {
		filename := noteFilename(timestamp, author, index)
		_, err := os.Stat(filepath.Join(notesDir, filename))
		if errors.Is(err, os.ErrNotExist) {
			return filename, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func colorText(out io.Writer, style, value string) string {
	return ui.ColorText(out, style, value)
}

func highlightMarkdown(out io.Writer, markdown string) string {
	return ui.HighlightMarkdown(out, markdown)
}
