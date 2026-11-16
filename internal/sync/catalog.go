package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type CatalogPushPlan struct {
	Kind      catdomain.Kind
	DeleteIDs []string
	Created   int
	Updated   int
	Deleted   int
}

func ReadLocalCatalogForSync(ws *workspace.Workspace, kind catdomain.Kind, baseline []map[string]any) ([]map[string]any, error) {
	local, err := ws.ReadCatalog(kind)
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
	local = catdomain.CloneRecords(baseline)
	if err := ws.WriteCatalog(kind, local); err != nil {
		return nil, err
	}
	return local, nil
}

func CatalogRecords(ctx context.Context, client *solutions.Client, projectID string, kind catdomain.Kind) ([]map[string]any, error) {
	if kind == catdomain.Labels {
		return client.ListLabels(ctx, projectID)
	}
	return client.ListPackages(ctx, projectID)
}

func PushProjectCatalog(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, kind catdomain.Kind) (map[string]string, *CatalogPushPlan, bool, error) {
	baseline, err := ws.ReadCatalogBaseline(kind)
	if err != nil {
		return nil, nil, false, err
	}
	local, err := ReadLocalCatalogForSync(ws, kind, baseline)
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
		return map[string]string{}, &CatalogPushPlan{Kind: kind}, false, nil
	}
	remoteRaw, err := CatalogRecords(ctx, client, project.ID, kind)
	if err != nil {
		return nil, nil, false, fmt.Errorf("fetch project %s before push: %w", kind, err)
	}
	remote, err := catdomain.NormalizeRecords(kind, remoteRaw)
	if err != nil {
		return nil, nil, false, err
	}
	merged, advanced, conflicts := catdomain.MergeRecords(kind, local, baseline, remote)
	if len(conflicts) > 0 {
		return nil, nil, false, fmt.Errorf("project %s conflict: %s; run `panda pull` and resolve the catalog locally", kind, strings.Join(conflicts, ", "))
	}
	remoteByID := make(map[string]map[string]any, len(remote))
	remoteRawByID := make(map[string]map[string]any, len(remoteRaw))
	for _, record := range remote {
		remoteByID[jsonutil.StringField(record, "id")] = record
	}
	for _, record := range remoteRaw {
		remoteRawByID[jsonutil.StringField(record, "id")] = record
	}
	localByID := make(map[string]map[string]any, len(merged))
	for _, record := range merged {
		if id := jsonutil.StringField(record, "id"); id != "" {
			localByID[id] = record
		}
	}
	plan := &CatalogPushPlan{Kind: kind}
	idMap := make(map[string]string)
	changed := false
	for _, base := range baseline {
		id := jsonutil.StringField(base, "id")
		if _, exists := localByID[id]; !exists && remoteByID[id] != nil {
			plan.DeleteIDs = append(plan.DeleteIDs, id)
			changed = true
		}
	}
	for index, record := range merged {
		localID := jsonutil.StringField(record, "localId")
		if localID != "" {
			for _, existing := range remoteByID {
				if strings.EqualFold(catdomain.RecordName(kind, existing), catdomain.RecordName(kind, record)) {
					return nil, plan, changed, fmt.Errorf("%s %q already exists remotely; run `panda pull` and resolve the duplicate", catdomain.ItemName(kind), catdomain.RecordName(kind, record))
				}
			}
			apiRecord, err := catdomain.RecordForAPI(kind, record)
			if err != nil {
				return nil, plan, changed, err
			}
			payload := CatalogAPIPayload(kind, apiRecord, project, true)
			createdRaw, err := CreateCatalogRecord(ctx, client, kind, payload)
			if err != nil {
				_ = ws.WriteCatalog(kind, merged)
				_ = ws.WriteCatalogBaseline(kind, advanced)
				return nil, plan, changed, fmt.Errorf("create %s %q: %w", catdomain.ItemName(kind), catdomain.RecordName(kind, record), err)
			}
			id := jsonutil.StringField(createdRaw, "id")
			if id == "" {
				return nil, plan, changed, fmt.Errorf("created %s %q but API response omitted its ID", catdomain.ItemName(kind), catdomain.RecordName(kind, record))
			}
			delete(record, "localId")
			createdRecords, err := catdomain.NormalizeRecords(kind, []map[string]any{createdRaw})
			if err != nil {
				return nil, plan, changed, err
			}
			created := jsonutil.CloneMap(record)
			for key, value := range createdRecords[0] {
				record[key] = value
				created[key] = value
			}
			idMap[localID] = id
			plan.Created++
			remoteByID[id] = created
			remoteRawByID[id] = jsonutil.CloneMap(createdRaw)
			advanced = append(advanced, jsonutil.CloneMap(created))
			changed = true
			if err := RewriteTaskCatalogIDs(ws, map[string]string{localID: id}); err != nil {
				return nil, plan, changed, err
			}
			if err := ws.WriteCatalog(kind, merged); err != nil {
				return nil, plan, changed, err
			}
			continue
		}
		id := jsonutil.StringField(record, "id")
		remoteRecord := remoteByID[id]
		if remoteRecord == nil {
			continue
		}
		needsUpdate := false
		payload := jsonutil.CloneMap(remoteRawByID[id])
		for _, field := range catdomain.Fields(kind) {
			if !jsonutil.Same(record[field], remoteRecord[field]) {
				if kind == catdomain.Packages && field == "description" {
					html, err := content.MarkdownToHTML(jsonutil.StringField(record, field))
					if err != nil {
						return nil, plan, changed, fmt.Errorf("convert package %q description to HTML: %w", catdomain.RecordName(kind, record), err)
					}
					payload[field] = html
				} else {
					catdomain.SetField(payload, field, record)
				}
				needsUpdate = true
			}
		}
		if !needsUpdate {
			continue
		}
		updatedRaw, err := UpdateCatalogRecord(ctx, client, kind, id, CatalogAPIPayload(kind, payload, project, false))
		if err != nil {
			_ = ws.WriteCatalog(kind, merged)
			_ = ws.WriteCatalogBaseline(kind, advanced)
			return nil, plan, changed, fmt.Errorf("update %s %q: %w", catdomain.ItemName(kind), catdomain.RecordName(kind, record), err)
		}
		if updatedRaw == nil {
			updatedRaw = payload
		}
		updated := jsonutil.CloneMap(record)
		updatedRecords, err := catdomain.NormalizeRecords(kind, []map[string]any{updatedRaw})
		if err != nil {
			return nil, plan, changed, err
		}
		for key, value := range updatedRecords[0] {
			updated[key] = value
		}
		merged[index] = jsonutil.CloneMap(updated)
		remoteByID[id] = jsonutil.CloneMap(updated)
		remoteRawByID[id] = jsonutil.CloneMap(updatedRaw)
		for i, baseRecord := range advanced {
			if jsonutil.StringField(baseRecord, "id") == id {
				advanced[i] = jsonutil.CloneMap(updated)
				break
			}
		}
		changed = true
		plan.Updated++
	}
	if err := ws.WriteCatalog(kind, merged); err != nil {
		return nil, plan, changed, err
	}
	filteredBaseline := make([]map[string]any, 0, len(advanced)+len(plan.DeleteIDs))
	for _, record := range advanced {
		if remoteByID[jsonutil.StringField(record, "id")] != nil {
			filteredBaseline = append(filteredBaseline, jsonutil.CloneMap(record))
		}
	}
	if err := ws.WriteCatalogBaseline(kind, filteredBaseline); err != nil {
		return nil, plan, changed, err
	}
	return idMap, plan, changed, nil
}

func CatalogAPIPayload(kind catdomain.Kind, record map[string]any, project *config.Project, create bool) map[string]any {
	payload := jsonutil.CloneMap(record)
	delete(payload, "localId")
	payload["project"] = map[string]any{"id": project.ID, "label": project.Name}
	if create {
		delete(payload, "id")
	}
	delete(payload, "label")
	delete(payload, "version")
	if kind == catdomain.Packages {
		delete(payload, "status")
		delete(payload, "tasks")
	}
	return payload
}

func CreateCatalogRecord(ctx context.Context, client *solutions.Client, kind catdomain.Kind, record map[string]any) (map[string]any, error) {
	if kind == catdomain.Labels {
		return client.CreateLabel(ctx, record)
	}
	return client.CreatePackage(ctx, record)
}

func UpdateCatalogRecord(ctx context.Context, client *solutions.Client, kind catdomain.Kind, id string, record map[string]any) (map[string]any, error) {
	if kind == catdomain.Labels {
		return client.UpdateLabel(ctx, id, record)
	}
	return client.UpdatePackage(ctx, id, record)
}

func PushProjectCatalogDeletes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, plans []*CatalogPushPlan) error {
	for _, plan := range plans {
		if plan == nil || len(plan.DeleteIDs) == 0 {
			continue
		}
		for _, id := range plan.DeleteIDs {
			var err error
			if plan.Kind == catdomain.Labels {
				err = client.DeleteLabel(ctx, id)
			} else {
				err = client.DeletePackage(ctx, id)
			}
			if err != nil {
				return fmt.Errorf("delete %s %s: %w", catdomain.ItemName(plan.Kind), id, err)
			}
			plan.Deleted++
		}
		remote, err := CatalogRecords(ctx, client, project.ID, plan.Kind)
		if err != nil {
			return fmt.Errorf("refresh project %s after deletes: %w", plan.Kind, err)
		}
		remote, err = catdomain.NormalizeRecords(plan.Kind, remote)
		if err != nil {
			return err
		}
		local, err := ws.ReadCatalog(plan.Kind)
		if err != nil {
			return err
		}
		baseline, err := ws.ReadCatalogBaseline(plan.Kind)
		if err != nil {
			return err
		}
		merged, advanced, conflicts := catdomain.MergeRecords(plan.Kind, local, baseline, remote)
		if len(conflicts) > 0 {
			return fmt.Errorf("project %s changed during push: %s; run `panda pull`", plan.Kind, strings.Join(conflicts, ", "))
		}
		if err := ws.WriteCatalog(plan.Kind, merged); err != nil {
			return err
		}
		if err := ws.WriteCatalogBaseline(plan.Kind, advanced); err != nil {
			return err
		}
	}
	return nil
}

func RemapTaskCatalogRelationships(task map[string]any, idMap map[string]string) (map[string]any, error) {
	result := jsonutil.CloneMap(task)
	if labels, ok := result["labels"].([]any); ok {
		mapped := make([]any, 0, len(labels))
		for _, value := range labels {
			relation, ok := value.(map[string]any)
			if !ok {
				mapped = append(mapped, value)
				continue
			}
			updated, err := mapCatalogRelation(relation, idMap)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, updated)
		}
		result["labels"] = mapped
	}
	if relation, ok := result["package"].(map[string]any); ok {
		updated, err := mapCatalogRelation(relation, idMap)
		if err != nil {
			return nil, err
		}
		result["package"] = updated
	}
	return result, nil
}

func RewriteTaskCatalogIDs(ws *workspace.Workspace, idMap map[string]string) error {
	if len(idMap) == 0 {
		return nil
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		path := filepath.Join(dir, "metadata.json")
		metadata, err := workspace.ReadMap(path)
		if err != nil {
			return err
		}
		updated := jsonutil.CloneMap(metadata)
		changed := false
		if labels, ok := updated["labels"].([]any); ok {
			mapped := make([]any, 0, len(labels))
			for _, value := range labels {
				relation, ok := value.(map[string]any)
				if ok {
					if remoteID, exists := idMap[jsonutil.RelationshipID(relation)]; exists {
						relation = jsonutil.CloneMap(relation)
						relation["id"] = remoteID
						changed = true
					}
					mapped = append(mapped, relation)
				} else {
					mapped = append(mapped, value)
				}
			}
			updated["labels"] = mapped
		}
		if relation, ok := updated["package"].(map[string]any); ok {
			if remoteID, exists := idMap[jsonutil.RelationshipID(relation)]; exists {
				relation = jsonutil.CloneMap(relation)
				relation["id"] = remoteID
				updated["package"] = relation
				changed = true
			}
		}
		if changed {
			if err := workspace.WriteMap(path, updated); err != nil {
				return err
			}
		}
	}
	return nil
}

func mapCatalogRelation(relation map[string]any, idMap map[string]string) (map[string]any, error) {
	updated := jsonutil.CloneMap(relation)
	id := jsonutil.RelationshipID(updated)
	if remoteID, exists := idMap[id]; exists {
		updated["id"] = remoteID
		return updated, nil
	}
	if strings.HasPrefix(id, "local-label-") || strings.HasPrefix(id, "local-package-") {
		return nil, fmt.Errorf("catalog item %q has not been pushed yet", report.RelationshipDisplayName(updated))
	}
	return updated, nil
}
