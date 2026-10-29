package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type catalogKind = catdomain.Kind

const (
	labelCatalog   catalogKind = catdomain.Labels
	packageCatalog catalogKind = catdomain.Packages
)

type catalogPushPlan struct {
	kind      catalogKind
	deleteIDs []string
}

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
	path := ws.CatalogPath(string(kind))
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

func writeCatalog(ws *workspace.Workspace, kind catalogKind, records []map[string]any) error {
	if records == nil {
		records = []map[string]any{}
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return workspace.WriteFileAtomic(ws.CatalogPath(string(kind)), append(data, '\n'), 0o600)
}

func readCatalogBaseline(ws *workspace.Workspace, kind catalogKind) ([]map[string]any, error) {
	state, err := ws.ReadState(ws.CatalogStateKey(string(kind)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, _ := state.Remote["records"].([]any)
	return mapsFromAny(items), nil
}

func writeCatalogBaseline(ws *workspace.Workspace, kind catalogKind, records []map[string]any) error {
	return ws.WriteState(ws.CatalogStateKey(string(kind)), workspace.State{
		Remote: map[string]any{"records": anyFromMaps(records)},
	})
}

func readLocalCatalogForSync(ws *workspace.Workspace, kind catalogKind, baseline []map[string]any) ([]map[string]any, error) {
	local, err := readCatalog(ws, kind)
	if err != nil {
		return nil, err
	}
	if local != nil {
		return local, nil
	}
	if _, err := os.Stat(ws.CatalogPath(string(kind))); err == nil {
		return local, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if len(baseline) == 0 {
		return local, nil
	}
	local = cloneCatalogRecords(baseline)
	if err := writeCatalog(ws, kind, local); err != nil {
		return nil, err
	}
	return local, nil
}

func cloneCatalogRecords(records []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(records))
	for _, record := range records {
		cloned = append(cloned, cloneMap(record))
	}
	return cloned
}

func mapsFromAny(items []any) []map[string]any {
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			records = append(records, cloneMap(record))
		}
	}
	return records
}

func anyFromMaps(records []map[string]any) []any {
	items := make([]any, 0, len(records))
	for _, record := range records {
		items = append(items, cloneMap(record))
	}
	return items
}

func catalogRecordID(record map[string]any) string {
	if id := stringField(record, "id"); id != "" {
		return id
	}
	return stringField(record, "localId")
}

func catalogRecordName(kind catalogKind, record map[string]any) string {
	return stringField(record, catalogNameField(kind))
}

func catalogRecords(ctx context.Context, client *solutions.Client, projectID string, kind catalogKind) ([]map[string]any, error) {
	if kind == labelCatalog {
		return client.ListLabels(ctx, projectID)
	}
	return client.ListPackages(ctx, projectID)
}

func normalizeCatalogRecords(kind catalogKind, records []map[string]any) ([]map[string]any, error) {
	normalized := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := cloneMap(record)
		if kind == packageCatalog {
			markdown, err := content.HTMLToMarkdown(stringField(record, "description"))
			if err != nil {
				return nil, fmt.Errorf("convert package %q description to Markdown: %w", catalogRecordName(kind, record), err)
			}
			item["description"] = markdown
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func catalogRecordForAPI(kind catalogKind, record map[string]any) (map[string]any, error) {
	item := cloneMap(record)
	if kind == packageCatalog {
		html, err := content.MarkdownToHTML(stringField(record, "description"))
		if err != nil {
			return nil, fmt.Errorf("convert package %q description to HTML: %w", catalogRecordName(kind, record), err)
		}
		item["description"] = html
	}
	return item, nil
}

func mergeCatalogRecords(kind catalogKind, local, baseline, remote []map[string]any) ([]map[string]any, []map[string]any, []string) {
	return catdomain.MergeRecords(kind, local, baseline, remote)
}

func setCatalogField(destination map[string]any, field string, source map[string]any) {
	if value, exists := source[field]; exists {
		destination[field] = value
	} else {
		delete(destination, field)
	}
}

func catalogRecordChanged(kind catalogKind, record, baseline map[string]any) bool {
	return catdomain.RecordChanged(kind, record, baseline)
}

func pushProjectCatalog(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, kind catalogKind) (map[string]string, *catalogPushPlan, bool, error) {
	baseline, err := readCatalogBaseline(ws, kind)
	if err != nil {
		return nil, nil, false, err
	}
	local, err := readLocalCatalogForSync(ws, kind, baseline)
	if err != nil {
		return nil, nil, false, err
	}
	_, fileErr := os.Stat(ws.CatalogPath(string(kind)))
	stateExists := false
	if _, stateErr := ws.ReadState(ws.CatalogStateKey(string(kind))); stateErr == nil {
		stateExists = true
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return nil, nil, false, stateErr
	}
	if errors.Is(fileErr, os.ErrNotExist) && !stateExists {
		return map[string]string{}, &catalogPushPlan{kind: kind}, false, nil
	}
	remoteRaw, err := catalogRecords(ctx, client, project.ID, kind)
	if err != nil {
		return nil, nil, false, fmt.Errorf("fetch project %s before push: %w", kind, err)
	}
	remote, err := normalizeCatalogRecords(kind, remoteRaw)
	if err != nil {
		return nil, nil, false, err
	}
	merged, advanced, conflicts := mergeCatalogRecords(kind, local, baseline, remote)
	if len(conflicts) > 0 {
		return nil, nil, false, fmt.Errorf("project %s conflict: %s; run `panda pull` and resolve the catalog locally", kind, strings.Join(conflicts, ", "))
	}
	remoteByID := make(map[string]map[string]any, len(remote))
	remoteRawByID := make(map[string]map[string]any, len(remoteRaw))
	for _, record := range remote {
		remoteByID[stringField(record, "id")] = record
	}
	for _, record := range remoteRaw {
		remoteRawByID[stringField(record, "id")] = record
	}
	localByID := make(map[string]map[string]any, len(merged))
	for _, record := range merged {
		if id := stringField(record, "id"); id != "" {
			localByID[id] = record
		}
	}
	plan := &catalogPushPlan{kind: kind}
	idMap := make(map[string]string)
	changed := false
	for _, base := range baseline {
		id := stringField(base, "id")
		if _, exists := localByID[id]; !exists && remoteByID[id] != nil {
			plan.deleteIDs = append(plan.deleteIDs, id)
			changed = true
		}
	}
	for index, record := range merged {
		localID := stringField(record, "localId")
		if localID != "" {
			for _, existing := range remoteByID {
				if strings.EqualFold(catalogRecordName(kind, existing), catalogRecordName(kind, record)) {
					return nil, plan, changed, fmt.Errorf("%s %q already exists remotely; run `panda pull` and resolve the duplicate", catalogItemName(kind), catalogRecordName(kind, record))
				}
			}
			apiRecord, err := catalogRecordForAPI(kind, record)
			if err != nil {
				return nil, plan, changed, err
			}
			payload := catalogAPIPayload(kind, apiRecord, project, true)
			createdRaw, err := createCatalogRecord(ctx, client, kind, payload)
			if err != nil {
				_ = writeCatalog(ws, kind, merged)
				_ = writeCatalogBaseline(ws, kind, advanced)
				return nil, plan, changed, fmt.Errorf("create %s %q: %w", catalogItemName(kind), catalogRecordName(kind, record), err)
			}
			id := stringField(createdRaw, "id")
			if id == "" {
				return nil, plan, changed, fmt.Errorf("created %s %q but API response omitted its ID", catalogItemName(kind), catalogRecordName(kind, record))
			}
			delete(record, "localId")
			createdRecords, err := normalizeCatalogRecords(kind, []map[string]any{createdRaw})
			if err != nil {
				return nil, plan, changed, err
			}
			created := cloneMap(record)
			for key, value := range createdRecords[0] {
				record[key] = value
				created[key] = value
			}
			idMap[localID] = id
			remoteByID[id] = created
			remoteRawByID[id] = cloneMap(createdRaw)
			advanced = append(advanced, cloneMap(created))
			changed = true
			if err := rewriteTaskCatalogIDs(ws, map[string]string{localID: id}); err != nil {
				return nil, plan, changed, err
			}
			if err := writeCatalog(ws, kind, merged); err != nil {
				return nil, plan, changed, err
			}
			continue
		}
		id := stringField(record, "id")
		remoteRecord := remoteByID[id]
		if remoteRecord == nil {
			continue
		}
		needsUpdate := false
		payload := cloneMap(remoteRawByID[id])
		for _, field := range catalogFields(kind) {
			if !same(valueAt(record, field), valueAt(remoteRecord, field)) {
				if kind == packageCatalog && field == "description" {
					html, err := content.MarkdownToHTML(stringField(record, field))
					if err != nil {
						return nil, plan, changed, fmt.Errorf("convert package %q description to HTML: %w", catalogRecordName(kind, record), err)
					}
					payload[field] = html
				} else {
					setCatalogField(payload, field, record)
				}
				needsUpdate = true
			}
		}
		if !needsUpdate {
			continue
		}
		updatedRaw, err := updateCatalogRecord(ctx, client, kind, id, catalogAPIPayload(kind, payload, project, false))
		if err != nil {
			_ = writeCatalog(ws, kind, merged)
			_ = writeCatalogBaseline(ws, kind, advanced)
			return nil, plan, changed, fmt.Errorf("update %s %q: %w", catalogItemName(kind), catalogRecordName(kind, record), err)
		}
		if updatedRaw == nil {
			updatedRaw = payload
		}
		updated := cloneMap(record)
		updatedRecords, err := normalizeCatalogRecords(kind, []map[string]any{updatedRaw})
		if err != nil {
			return nil, plan, changed, err
		}
		for key, value := range updatedRecords[0] {
			updated[key] = value
		}
		merged[index] = cloneMap(updated)
		remoteByID[id] = cloneMap(updated)
		remoteRawByID[id] = cloneMap(updatedRaw)
		for i, baseRecord := range advanced {
			if stringField(baseRecord, "id") == id {
				advanced[i] = cloneMap(updated)
				break
			}
		}
		changed = true
	}
	if err := writeCatalog(ws, kind, merged); err != nil {
		return nil, plan, changed, err
	}
	// Keep remotely deleted records out of the baseline; local deletions remain
	// in it until their API deletes have succeeded.
	filteredBaseline := make([]map[string]any, 0, len(advanced)+len(plan.deleteIDs))
	for _, record := range advanced {
		if remoteByID[stringField(record, "id")] != nil {
			filteredBaseline = append(filteredBaseline, cloneMap(record))
		}
	}
	if err := writeCatalogBaseline(ws, kind, filteredBaseline); err != nil {
		return nil, plan, changed, err
	}
	return idMap, plan, changed, nil
}

func catalogAPIPayload(kind catalogKind, record map[string]any, project *config.Project, create bool) map[string]any {
	payload := cloneMap(record)
	delete(payload, "localId")
	payload["project"] = map[string]any{"id": project.ID, "label": project.Name}
	if create {
		delete(payload, "id")
	}
	delete(payload, "label")
	delete(payload, "version")
	if kind == packageCatalog {
		delete(payload, "status")
		delete(payload, "tasks")
	}
	return payload
}

func createCatalogRecord(ctx context.Context, client *solutions.Client, kind catalogKind, record map[string]any) (map[string]any, error) {
	if kind == labelCatalog {
		return client.CreateLabel(ctx, record)
	}
	return client.CreatePackage(ctx, record)
}

func updateCatalogRecord(ctx context.Context, client *solutions.Client, kind catalogKind, id string, record map[string]any) (map[string]any, error) {
	if kind == labelCatalog {
		return client.UpdateLabel(ctx, id, record)
	}
	return client.UpdatePackage(ctx, id, record)
}

func pushProjectCatalogDeletes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, plans []*catalogPushPlan) error {
	for _, plan := range plans {
		if plan == nil || len(plan.deleteIDs) == 0 {
			continue
		}
		for _, id := range plan.deleteIDs {
			var err error
			if plan.kind == labelCatalog {
				err = client.DeleteLabel(ctx, id)
			} else {
				err = client.DeletePackage(ctx, id)
			}
			if err != nil {
				return fmt.Errorf("delete %s %s: %w", catalogItemName(plan.kind), id, err)
			}
		}
		remote, err := catalogRecords(ctx, client, project.ID, plan.kind)
		if err != nil {
			return fmt.Errorf("refresh project %s after deletes: %w", plan.kind, err)
		}
		remote, err = normalizeCatalogRecords(plan.kind, remote)
		if err != nil {
			return err
		}
		local, err := readCatalog(ws, plan.kind)
		if err != nil {
			return err
		}
		baseline, err := readCatalogBaseline(ws, plan.kind)
		if err != nil {
			return err
		}
		merged, advanced, conflicts := mergeCatalogRecords(plan.kind, local, baseline, remote)
		if len(conflicts) > 0 {
			return fmt.Errorf("project %s changed during push: %s; run `panda pull`", plan.kind, strings.Join(conflicts, ", "))
		}
		if err := writeCatalog(ws, plan.kind, merged); err != nil {
			return err
		}
		if err := writeCatalogBaseline(ws, plan.kind, advanced); err != nil {
			return err
		}
	}
	return nil
}
