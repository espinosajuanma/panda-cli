package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type browserUI interface {
	Select(prompt string, options []selectorOption, selected string) (string, error)
}

type terminalBrowserUI struct {
	in  io.Reader
	out io.Writer
}

func (browser terminalBrowserUI) Select(prompt string, options []selectorOption, selected string) (string, error) {
	return selectSingleOption(browser.in, browser.out, prompt, options, selected)
}

func readOnlyProjectWorkspace(cfg *config.Config) (*workspace.Workspace, error) {
	if cfg.Project == nil || cfg.Project.ID == "" || cfg.Project.Name == "" {
		return nil, errors.New("no project selected; run `panda config set project <name>` first")
	}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("project workspace does not exist; run `panda pull` first")
		}
		return nil, fmt.Errorf("inspect project workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project workspace path %q is not a directory", root)
	}
	return workspace.New(root), nil
}

func browseTasks(ws *workspace.Workspace, args []string, userEmail, apiURL string, in io.Reader, out io.Writer) error {
	if !isInteractiveTerminal(in, out) {
		return errors.New("panda browse requires an interactive terminal; use `panda list` for non-interactive output")
	}
	return browseWithUI(ws, args, userEmail, apiURL, terminalBrowserUI{in: in, out: out}, out)
}

func browseWithUI(ws *workspace.Workspace, args []string, userEmail, apiURL string, browser browserUI, out io.Writer) error {
	options, err := parseListOptions(args)
	if err != nil {
		if strings.HasPrefix(err.Error(), "usage: panda list") {
			return errors.New(strings.Replace(err.Error(), "panda list", "panda browse", 1))
		}
		return err
	}
	if options.json {
		return errors.New("panda browse is interactive and does not accept --json")
	}
	for {
		report, err := collectListReportForUser(ws, options, userEmail)
		if err != nil {
			return err
		}
		choices, entries := browserTaskChoices(report)
		if len(choices) == 0 {
			fmt.Fprintln(out, "No tasks match the filters.")
			return nil
		}
		selected, err := browser.Select("Tasks · type to filter", choices, "")
		if errors.Is(err, errSelectionCanceled) {
			return nil
		}
		if err != nil {
			return err
		}
		entry, ok := entries[selected]
		if !ok {
			return fmt.Errorf("selected task %q is no longer in the workspace", selected)
		}
	actionLoop:
		for {
			actions := browserTaskActions(ws, entry, apiURL)
			action, selectErr := browser.Select("Task actions", actions, "view")
			if errors.Is(selectErr, errSelectionCanceled) {
				return nil
			}
			if selectErr != nil {
				return selectErr
			}
			switch action {
			case "back":
				break actionLoop
			case "quit":
				return nil
			default:
				if err := runBrowserTaskAction(ws, entry, action, apiURL, out); err != nil {
					fmt.Fprintf(out, "Unable to %s: %v\n\n", browserActionLabel(actions, action), err)
				}
			}
		}
	}
}

func browserTaskChoices(report listReport) ([]selectorOption, map[string]listTaskEntry) {
	var choices []selectorOption
	entries := make(map[string]listTaskEntry)
	for _, group := range report.groups {
		for _, entry := range group.entries {
			selector := filepath.Base(entry.dir)
			label := fmt.Sprintf("%s [%s] %s", taskLabel(entry.metadata), displayStatus(group.status), tableCell(entry.metadata["title"]))
			if assignees := assigneesDisplay(entry.metadata["assignees"]); assignees != "" {
				label += " · " + tableCell(assignees)
			}
			choices = append(choices, selectorOption{value: selector, label: label})
			entries[selector] = entry
		}
	}
	return choices, entries
}

func browserTaskActions(ws *workspace.Workspace, entry listTaskEntry, apiURL string) []selectorOption {
	actions := []selectorOption{
		{value: "view", label: "View full task"},
		{value: "description", label: "Read description"},
		{value: "notes", label: "Read notes"},
		{value: "files", label: "View attached files"},
		{value: "diff", label: "Review pending diff"},
	}
	if _, err := verifiedTaskLinkForAPIURL(ws, entry.metadata, apiURL); err == nil {
		actions = append(actions, selectorOption{value: "link", label: "Print task link"})
	}
	actions = append(actions,
		selectorOption{value: "back", label: "Back to tasks"},
		selectorOption{value: "quit", label: "Quit browser"},
	)
	return actions
}

func browserActionLabel(actions []selectorOption, value string) string {
	for _, action := range actions {
		if action.value == value {
			return strings.ToLower(action.label)
		}
	}
	return "show task information"
}

func runBrowserTaskAction(ws *workspace.Workspace, entry listTaskEntry, action, apiURL string, out io.Writer) error {
	selector := filepath.Base(entry.dir)
	switch action {
	case "view":
		return viewTaskWithAPIURL(ws, []string{selector}, apiURL, out)
	case "description":
		return renderBrowseDescription(entry, out)
	case "notes":
		return renderBrowseNotes(ws, entry, out)
	case "files":
		return filesCommand(ws, []string{selector}, nil, out)
	case "diff":
		return diff(ws, []string{selector, "--verbose"}, out)
	case "link":
		return linkTaskWithAPIURL(ws, []string{selector}, apiURL, out)
	default:
		return fmt.Errorf("unknown task browser action %q", action)
	}
}

func renderBrowseDescription(entry listTaskEntry, out io.Writer) error {
	data, err := os.ReadFile(filepath.Join(entry.dir, "description.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			data = []byte("No description.")
		} else {
			return err
		}
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		fmt.Fprintf(rendered, "%s\n\n", colorText(out, "1;32", "## Description — #"+taskLabel(entry.metadata)))
		fmt.Fprintln(rendered, highlightMarkdown(out, string(data)))
		return nil
	})
}

func renderBrowseNotes(ws *workspace.Workspace, entry listTaskEntry, out io.Writer) error {
	state, err := ws.ReadState(stateKey(entry.dir, entry.metadata))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	notes, err := readTaskViewNotes(entry.dir, state)
	if err != nil {
		return err
	}
	return writePagedOutput(out, func(rendered io.Writer) error {
		fmt.Fprintf(rendered, "%s\n\n", colorText(out, "1;32", "## Notes — #"+taskLabel(entry.metadata)))
		if len(notes) == 0 {
			fmt.Fprintln(rendered, "No notes.")
			return nil
		}
		for _, note := range notes {
			marker := ""
			if note.Staged {
				marker = " [staged]"
			}
			fmt.Fprintf(rendered, "%s%s\n", colorText(out, "1;36", "### "+strings.TrimSuffix(note.Filename, ".md")), marker)
			fmt.Fprintln(rendered, highlightMarkdown(out, note.Markdown))
		}
		return nil
	})
}
