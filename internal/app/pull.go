package app

import (
	"context"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type pullProgress = syncpkg.PullProgress

func pull(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	return syncpkg.Pull(ctx, client, ws, project, out)
}

func writeProjectMembers(ws *workspace.Workspace, members []map[string]any) error {
	return syncpkg.WriteProjectMembers(ws, members)
}

func dedupeProjectMembers(members []map[string]any) []map[string]any {
	return syncpkg.DedupeProjectMembers(members)
}

func sameTaskVersion(remote, baseline map[string]any) bool {
	return syncpkg.SameTaskVersion(remote, baseline)
}

func newPullProgress(out io.Writer) *pullProgress {
	return syncpkg.NewPullProgress(out)
}

func formatPullProgress(percent int) string {
	return syncpkg.FormatPullProgress(percent)
}
