package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func editTaskStatusField(metadata map[string]any, in io.Reader, out io.Writer) error {
	current := stringField(metadata, "status")
	if isInteractiveTerminal(in, out) {
		choices := make([]selectorOption, 0, len(task.StatusOrder))
		for _, status := range task.StatusOrder {
			choices = append(choices, selectorOption{value: status, label: status})
		}
		selected, err := selectSingleOption(in, out, "Status", choices, current)
		if err != nil {
			return err
		}
		metadata["status"] = selected
		return nil
	}

	choice, err := prompt(in, out, fmt.Sprintf("%s [%s] (%s): ", colorText(out, "1", "Status"), current, strings.Join(task.StatusOrder, "/")))
	if err != nil {
		return err
	}
	if choice == "" {
		return nil
	}
	if !contains(task.StatusOrder, choice) {
		return fmt.Errorf("invalid status %q; choose one of %s", choice, strings.Join(task.StatusOrder, ", "))
	}
	metadata["status"] = choice
	return nil
}

// saveTaskStatus records status changes locally and stages the dedicated remote
// discard action when a synced task is moved into the discarded status.
func saveTaskStatus(ws *workspace.Workspace, dir string, metadata map[string]any, status string) error {
	state, key, err := loadTaskState(ws, dir, metadata)
	if err != nil {
		return err
	}
	state.StagedDiscard = status == "discarded" && !state.LocalCreated && stringField(state.Remote, "status") != "discarded"
	if err := ws.WriteState(key, state); err != nil {
		return err
	}
	metadata["status"] = status
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return fmt.Errorf("save task status: %w", err)
	}
	return nil
}
