package app

import (
	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type catalogKind = catdomain.Kind

const (
	labelCatalog   catalogKind = catdomain.Labels
	packageCatalog catalogKind = catdomain.Packages
)

func catalogItemName(kind catalogKind) string {
	return catdomain.ItemName(kind)
}

func catalogNameField(kind catalogKind) string {
	return catdomain.NameField(kind)
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

func cloneCatalogRecords(records []map[string]any) []map[string]any {
	return catdomain.CloneRecords(records)
}

func catalogRecordID(record map[string]any) string {
	return catdomain.RecordID(record)
}

func catalogRecordName(kind catalogKind, record map[string]any) string {
	return catdomain.RecordName(catdomain.Kind(kind), record)
}

func catalogRecordChanged(kind catalogKind, record, baseline map[string]any) bool {
	return catdomain.RecordChanged(kind, record, baseline)
}
