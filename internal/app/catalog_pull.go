package app

import (
	"context"
	"io"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func projectCatalogDiff(ws *workspace.Workspace) ([]diffCatalogChange, int, int, error) {
	changes, labelChanges, packageChanges, err := syncpkg.CatalogDiff(ws)
	if err != nil {
		return nil, 0, 0, err
	}
	result := make([]diffCatalogChange, 0, len(changes))
	for _, change := range changes {
		result = append(result, diffCatalogChange{Kind: change.Kind, Action: change.Action, Name: change.Name, Fields: change.Fields})
	}
	return result, labelChanges, packageChanges, nil
}

func pullProjectCatalogs(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) (int, error) {
	return syncpkg.PullProjectCatalogs(ctx, client, ws, project, out)
}

func pullProjectCatalogsFromRecords(ws *workspace.Workspace, records map[catalogKind][]map[string]any, out io.Writer, progress *pullProgress) (int, error) {
	converted := make(map[catdomain.Kind][]map[string]any, len(records))
	for kind, kindRecords := range records {
		converted[catdomain.Kind(kind)] = kindRecords
	}
	return syncpkg.PullProjectCatalogsFromRecords(ws, converted, out, progress)
}

func dedupeCatalogRecords(records []map[string]any) []map[string]any {
	return syncpkg.DedupeCatalogRecords(records)
}
