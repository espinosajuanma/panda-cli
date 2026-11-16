package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func (w *Workspace) ReadCatalog(kind catalog.Kind) ([]map[string]any, error) {
	path := w.CatalogPath(string(kind))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return records, nil
}

func (w *Workspace) WriteCatalog(kind catalog.Kind, records []map[string]any) error {
	if records == nil {
		records = []map[string]any{}
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(w.CatalogPath(string(kind)), append(data, '\n'), 0o600)
}

func (w *Workspace) ReadCatalogBaseline(kind catalog.Kind) ([]map[string]any, error) {
	state, err := w.ReadState(w.CatalogStateKey(string(kind)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, _ := state.Remote["records"].([]any)
	return mapsFromAny(items), nil
}

func (w *Workspace) WriteCatalogBaseline(kind catalog.Kind, records []map[string]any) error {
	return w.WriteState(w.CatalogStateKey(string(kind)), State{
		Remote: map[string]any{"records": anyFromMaps(records)},
	})
}

func mapsFromAny(items []any) []map[string]any {
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			records = append(records, jsonutil.CloneMap(record))
		}
	}
	return records
}

func anyFromMaps(records []map[string]any) []any {
	items := make([]any, 0, len(records))
	for _, record := range records {
		items = append(items, jsonutil.CloneMap(record))
	}
	return items
}
