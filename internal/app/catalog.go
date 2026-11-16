package app

import (
	"context"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type catalogKind = catdomain.Kind

const (
	labelCatalog   catalogKind = catdomain.Labels
	packageCatalog catalogKind = catdomain.Packages
)

type catalogPushPlan = syncpkg.CatalogPushPlan

func catalogItemName(kind catalogKind) string {
	return catdomain.ItemName(kind)
}

func catalogNameField(kind catalogKind) string {
	return catdomain.NameField(kind)
}

func catalogFields(kind catalogKind) []string {
	return catdomain.Fields(kind)
}

func readCatalog(ws *workspace.Workspace, kind catalogKind) ([]map[string]any, error) {
	return ws.ReadCatalog(catdomain.Kind(kind))
}

func writeCatalog(ws *workspace.Workspace, kind catalogKind, records []map[string]any) error {
	return ws.WriteCatalog(catdomain.Kind(kind), records)
}

func readCatalogBaseline(ws *workspace.Workspace, kind catalogKind) ([]map[string]any, error) {
	return ws.ReadCatalogBaseline(catdomain.Kind(kind))
}

func writeCatalogBaseline(ws *workspace.Workspace, kind catalogKind, records []map[string]any) error {
	return ws.WriteCatalogBaseline(catdomain.Kind(kind), records)
}

func readLocalCatalogForSync(ws *workspace.Workspace, kind catalogKind, baseline []map[string]any) ([]map[string]any, error) {
	return syncpkg.ReadLocalCatalogForSync(ws, catdomain.Kind(kind), baseline)
}

func cloneCatalogRecords(records []map[string]any) []map[string]any {
	return catdomain.CloneRecords(records)
}

func catalogRecordID(record map[string]any) string {
	return catdomain.RecordID(record)
}

func catalogRecordName(kind catalogKind, record map[string]any) string {
	return catdomain.RecordName(catdomain.Kind(kind), record)
}

func catalogRecords(ctx context.Context, client *solutions.Client, projectID string, kind catalogKind) ([]map[string]any, error) {
	return syncpkg.CatalogRecords(ctx, client, projectID, catdomain.Kind(kind))
}

func normalizeCatalogRecords(kind catalogKind, records []map[string]any) ([]map[string]any, error) {
	return catdomain.NormalizeRecords(catdomain.Kind(kind), records)
}

func catalogRecordForAPI(kind catalogKind, record map[string]any) (map[string]any, error) {
	return catdomain.RecordForAPI(catdomain.Kind(kind), record)
}

func mergeCatalogRecords(kind catalogKind, local, baseline, remote []map[string]any) ([]map[string]any, []map[string]any, []string) {
	return catdomain.MergeRecords(kind, local, baseline, remote)
}

func setCatalogField(destination map[string]any, field string, source map[string]any) {
	catdomain.SetField(destination, field, source)
}

func catalogRecordChanged(kind catalogKind, record, baseline map[string]any) bool {
	return catdomain.RecordChanged(kind, record, baseline)
}

func pushProjectCatalog(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, kind catalogKind) (map[string]string, *catalogPushPlan, bool, error) {
	return syncpkg.PushProjectCatalog(ctx, client, ws, project, catdomain.Kind(kind))
}

func pushProjectCatalogDeletes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, plans []*catalogPushPlan) error {
	return syncpkg.PushProjectCatalogDeletes(ctx, client, ws, project, plans)
}
