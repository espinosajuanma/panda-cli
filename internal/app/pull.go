package app

import (
	"context"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func pull(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	return syncpkg.Pull(ctx, client, ws, project, out)
}

func writeProjectMembers(ws *workspace.Workspace, members []map[string]any) error {
	return syncpkg.WriteProjectMembers(ws, members)
}
