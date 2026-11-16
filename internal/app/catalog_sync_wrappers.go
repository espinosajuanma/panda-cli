package app

import (
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func remapTaskCatalogRelationships(task map[string]any, idMap map[string]string) (map[string]any, error) {
	return syncpkg.RemapTaskCatalogRelationships(task, idMap)
}

func rewriteTaskCatalogIDs(ws *workspace.Workspace, idMap map[string]string) error {
	return syncpkg.RewriteTaskCatalogIDs(ws, idMap)
}
