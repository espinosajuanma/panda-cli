package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type rankedTask struct {
	dir      string
	metadata map[string]any
}

func moveTaskRank(ws *workspace.Workspace, selectedDir string, selected map[string]any, direction string, out io.Writer) error {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "up" && direction != "down" {
		return fmt.Errorf("invalid rank direction %q; choose up or down", direction)
	}

	status := stringField(selected, "status")
	if status == "" {
		status = "new"
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	group := make([]rankedTask, 0)
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return fmt.Errorf("read task metadata in %s: %w", filepath.Base(dir), err)
		}
		taskStatus := stringField(metadata, "status")
		if taskStatus == "" {
			taskStatus = "new"
		}
		if taskStatus == status {
			group = append(group, rankedTask{dir: dir, metadata: metadata})
		}
	}
	sort.SliceStable(group, func(i, j int) bool {
		return taskMetadataRankLess(group[i].metadata, group[j].metadata)
	})

	index := -1
	for i := range group {
		if group[i].dir == selectedDir {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("task was not found in its status group")
	}
	neighborIndex := index - 1
	if direction == "down" {
		neighborIndex = index + 1
	}
	if neighborIndex < 0 || neighborIndex >= len(group) {
		return fmt.Errorf("%s is already %s in status %s", taskLabel(selected), map[string]string{"up": "first", "down": "last"}[direction], status)
	}

	current := group[index]
	neighbor := group[neighborIndex]
	currentRank := stringField(current.metadata, "rank")
	neighborRank := stringField(neighbor.metadata, "rank")
	if currentRank == "" || neighborRank == "" {
		return errors.New("cannot reorder tasks without fetched ranks; run `panda pull` first")
	}
	if same(current.metadata["rank"], neighbor.metadata["rank"]) {
		return errors.New("cannot reorder tasks with equal ranks; run `panda pull` first")
	}

	current.metadata["rank"], neighbor.metadata["rank"] = neighbor.metadata["rank"], current.metadata["rank"]
	if err := workspace.WriteMap(filepath.Join(current.dir, "metadata.json"), current.metadata); err != nil {
		return fmt.Errorf("save rank for %s: %w", taskLabel(current.metadata), err)
	}
	if err := workspace.WriteMap(filepath.Join(neighbor.dir, "metadata.json"), neighbor.metadata); err != nil {
		return fmt.Errorf("save rank for %s: %w", taskLabel(neighbor.metadata), err)
	}
	fmt.Fprintf(out, "Moved %s %s, swapping rank with %s. Run `panda push` to publish both changes.\n", taskLabel(selected), direction, taskLabel(neighbor.metadata))
	return nil
}
