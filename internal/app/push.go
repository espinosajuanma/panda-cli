package app

import (
	"context"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func push(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	return syncpkg.Push(ctx, client, ws, project, out)
}

func pushSelected(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, selectors []string, out io.Writer) error {
	return syncpkg.PushSelected(ctx, client, ws, project, selectors, out)
}

func pushSelectedWithOptions(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, options pushOptions, out io.Writer) error {
	return syncpkg.PushSelectedWithOptions(ctx, client, ws, project, syncpkg.PushOptions{
		Selectors: options.selectors,
		JSON:      options.json,
	}, out)
}

func recoverCreatedTask(ws *workspace.Workspace, dir string, previous map[string]any, oldKey string, state workspace.State) (string, map[string]any, string, error) {
	return syncpkg.RecoverCreatedTask(ws, dir, previous, oldKey, state)
}

func taskHasLocalChanges(local map[string]any, state workspace.State, dir string) (bool, error) {
	return syncpkg.TaskHasLocalChanges(local, state, dir)
}
