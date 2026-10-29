package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/content"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

type catalogKind string

const (
	labelCatalog   catalogKind = "labels"
	packageCatalog catalogKind = "packages"
)

type catalogPushPlan struct {
	kind      catalogKind
	deleteIDs []string
}

func catalogItemName(kind catalogKind) string {
	if kind == labelCatalog {
		return "label"
	}
	return "package"
}

func catalogNameField(kind catalogKind) string {
	if kind == labelCatalog {
		return "name"
	}
	return "title"
}

func catalogFields(kind catalogKind) []string {
	if kind == labelCatalog {
		return []string{"name", "description"}
	}
	return []string{"title", "description", "startDate", "estimation"}
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
	fields := catalogFields(kind)
	localByID := make(map[string]map[string]any)
	remoteByID := make(map[string]map[string]any)
	for _, record := range local {
		if id := stringField(record, "id"); id != "" {
			localByID[id] = record
		}
	}
	for _, record := range remote {
		if id := stringField(record, "id"); id != "" {
			remoteByID[id] = record
		}
	}

	merged := make([]map[string]any, 0, len(local)+len(remote))
	advanced := make([]map[string]any, 0, len(remote)+len(baseline))
	conflicts := make([]string, 0)
	handledLocal := make(map[string]bool)
	handledRemote := make(map[string]bool)
	for _, base := range baseline {
		id := stringField(base, "id")
		localRecord, hasLocal := localByID[id]
		remoteRecord, hasRemote := remoteByID[id]
		switch {
		case !hasLocal && !hasRemote:
			continue
		case !hasLocal && hasRemote:
			if catalogRecordChanged(kind, remoteRecord, base) {
				conflicts = append(conflicts, fmt.Sprintf("%s %q was deleted locally but changed remotely", catalogItemName(kind), catalogRecordName(kind, base)))
			}
			advanced = append(advanced, cloneMap(base))
			handledRemote[id] = true
		case hasLocal && !hasRemote:
			if catalogRecordChanged(kind, localRecord, base) {
				conflicts = append(conflicts, fmt.Sprintf("%s %q was deleted remotely but changed locally", catalogItemName(kind), catalogRecordName(kind, base)))
				merged = append(merged, cloneMap(localRecord))
				advanced = append(advanced, cloneMap(base))
			}
			handledLocal[id] = true
		case hasLocal && hasRemote:
			updatedLocal := cloneMap(remoteRecord)
			updatedBaseline := cloneMap(remoteRecord)
			pending := false
			for _, field := range fields {
				localChanged := !same(valueAt(localRecord, field), valueAt(base, field))
				remoteChanged := !same(valueAt(remoteRecord, field), valueAt(base, field))
				if !localChanged {
					continue
				}
				localValue, remoteValue := valueAt(localRecord, field), valueAt(remoteRecord, field)
				if remoteChanged && !same(localValue, remoteValue) {
					conflicts = append(conflicts, fmt.Sprintf("%s %q: %s", catalogItemName(kind), catalogRecordName(kind, base), field))
				}
				if same(localValue, remoteValue) {
					continue
				}
				setCatalogField(updatedLocal, field, localRecord)
				setCatalogField(updatedBaseline, field, base)
				pending = true
			}
			merged = append(merged, updatedLocal)
			if pending {
				advanced = append(advanced, updatedBaseline)
			} else {
				advanced = append(advanced, cloneMap(remoteRecord))
			}
			handledLocal[id] = true
			handledRemote[id] = true
		}
	}
	for _, record := range remote {
		id := stringField(record, "id")
		if id != "" && handledRemote[id] {
			continue
		}
		merged = append(merged, cloneMap(record))
		advanced = append(advanced, cloneMap(record))
		handledRemote[id] = true
	}
	for _, record := range local {
		if stringField(record, "localId") != "" {
			merged = append(merged, cloneMap(record))
			continue
		}
		id := stringField(record, "id")
		if id == "" || !handledLocal[id] {
			merged = append(merged, cloneMap(record))
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return strings.ToLower(catalogRecordName(kind, merged[i])) < strings.ToLower(catalogRecordName(kind, merged[j]))
	})
	sort.Strings(conflicts)
	return merged, advanced, conflicts
}

func setCatalogField(destination map[string]any, field string, source map[string]any) {
	if value, exists := source[field]; exists {
		destination[field] = value
	} else {
		delete(destination, field)
	}
}

func catalogRecordChanged(kind catalogKind, record, baseline map[string]any) bool {
	for _, field := range catalogFields(kind) {
		if !same(valueAt(record, field), valueAt(baseline, field)) {
			return true
		}
	}
	return false
}

func projectCatalogDiff(ws *workspace.Workspace) ([]string, int, int, error) {
	var lines []string
	labelChanges, packageChanges := 0, 0
	for _, kind := range []catalogKind{labelCatalog, packageCatalog} {
		local, err := readCatalog(ws, kind)
		if err != nil {
			return nil, 0, 0, err
		}
		baseline, err := readCatalogBaseline(ws, kind)
		if err != nil {
			return nil, 0, 0, err
		}
		baseByID := make(map[string]map[string]any, len(baseline))
		localByID := make(map[string]map[string]any, len(local))
		for _, record := range baseline {
			baseByID[stringField(record, "id")] = record
		}
		for _, record := range local {
			if id := stringField(record, "id"); id != "" {
				localByID[id] = record
			}
		}
		for _, record := range local {
			name := catalogRecordName(kind, record)
			if stringField(record, "localId") != "" {
				lines = append(lines, fmt.Sprintf("  new %s: %s", catalogItemName(kind), name))
				if kind == labelCatalog {
					labelChanges++
				} else {
					packageChanges++
				}
				continue
			}
			base := baseByID[stringField(record, "id")]
			if base == nil {
				lines = append(lines, fmt.Sprintf("  new %s: %s", catalogItemName(kind), name))
				if kind == labelCatalog {
					labelChanges++
				} else {
					packageChanges++
				}
				continue
			}
			var changed []string
			for _, field := range catalogFields(kind) {
				if !same(valueAt(record, field), valueAt(base, field)) {
					changed = append(changed, field)
				}
			}
			if len(changed) > 0 {
				lines = append(lines, fmt.Sprintf("  edited %s: %s (%s)", catalogItemName(kind), name, strings.Join(changed, ", ")))
				if kind == labelCatalog {
					labelChanges++
				} else {
					packageChanges++
				}
			}
		}
		for _, record := range baseline {
			id := stringField(record, "id")
			if id != "" && localByID[id] == nil {
				lines = append(lines, fmt.Sprintf("  deleted %s: %s", catalogItemName(kind), catalogRecordName(kind, record)))
				if kind == labelCatalog {
					labelChanges++
				} else {
					packageChanges++
				}
			}
		}
	}
	sort.Strings(lines)
	return lines, labelChanges, packageChanges, nil
}

func pullProjectCatalogs(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) (int, error) {
	conflicts := 0
	counts := make(map[catalogKind]int, 2)
	for _, kind := range []catalogKind{labelCatalog, packageCatalog} {
		remote, err := catalogRecords(ctx, client, project.ID, kind)
		if err != nil {
			return conflicts, fmt.Errorf("pull project %s: %w", kind, err)
		}
		remote, err = normalizeCatalogRecords(kind, remote)
		if err != nil {
			return conflicts, err
		}
		baseline, err := readCatalogBaseline(ws, kind)
		if err != nil {
			return conflicts, err
		}
		local, err := readLocalCatalogForSync(ws, kind, baseline)
		if err != nil {
			return conflicts, err
		}
		merged, advanced, issues := mergeCatalogRecords(kind, local, baseline, remote)
		for _, issue := range issues {
			conflicts++
			fmt.Fprintf(out, "%s project %s: %s\n", colorText(out, "1;31", "Conflict"), kind, issue)
		}
		if err := writeCatalog(ws, kind, merged); err != nil {
			return conflicts, err
		}
		if err := writeCatalogBaseline(ws, kind, advanced); err != nil {
			return conflicts, err
		}
		counts[kind] = len(merged)
	}
	fmt.Fprintf(out, "Synchronized project catalogs: %d labels, %d packages.\n", counts[labelCatalog], counts[packageCatalog])
	return conflicts, nil
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

func projectCatalogCommand(ws *workspace.Workspace, project *config.Project, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
		return listCatalog(ws, kind, out)
	}
	switch args[0] {
	case "create":
		return createCatalogRecordLocal(ws, project, kind, args[1:], in, out)
	case "edit":
		return editCatalogRecordLocal(ws, kind, args[1:], in, out)
	case "delete", "remove":
		return deleteCatalogRecordLocal(ws, kind, args[1:], out)
	default:
		return fmt.Errorf("usage: panda %s [list | create [name|title] [description] | edit <name|title> | delete <name|title>]", kind)
	}
}

func listCatalog(ws *workspace.Workspace, kind catalogKind, out io.Writer) error {
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	baseline, err := readCatalogBaseline(ws, kind)
	if err != nil {
		return err
	}
	baseByID := make(map[string]map[string]any, len(baseline))
	for _, record := range baseline {
		baseByID[stringField(record, "id")] = record
	}
	if kind == labelCatalog {
		fmt.Fprintln(out, "Project labels")
	} else {
		fmt.Fprintln(out, "Project packages")
	}
	for _, record := range records {
		status := ""
		if stringField(record, "localId") != "" {
			status = " [new]"
		} else if base := baseByID[stringField(record, "id")]; base != nil && catalogRecordChanged(kind, record, base) {
			status = " [edited]"
		}
		name := catalogRecordName(kind, record)
		if description := strings.TrimSpace(stringField(record, "description")); description != "" {
			fmt.Fprintf(out, "  %s%s — %s\n", name, status, description)
		} else {
			fmt.Fprintf(out, "  %s%s\n", name, status)
		}
	}
	deleted := 0
	localIDs := make(map[string]bool)
	for _, record := range records {
		localIDs[stringField(record, "id")] = true
	}
	for _, record := range baseline {
		if !localIDs[stringField(record, "id")] {
			deleted++
		}
	}
	if deleted > 0 {
		fmt.Fprintf(out, "  %d deletion(s) staged\n", deleted)
	}
	if len(records) == 0 && deleted == 0 {
		fmt.Fprintf(out, "  No project %s.\n", kind)
	}
	return nil
}

func createCatalogRecordLocal(ws *workspace.Workspace, project *config.Project, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	name := ""
	description := ""
	supplemental := make(map[string]any)
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
		description = strings.TrimSpace(strings.Join(args[1:], " "))
	} else {
		var err error
		nameFieldTitle := catalogNameField(kind)
		name, err = prompt(in, out, fmt.Sprintf("%s: ", colorText(out, "1", strings.ToUpper(nameFieldTitle[:1])+nameFieldTitle[1:])))
		if err != nil {
			return err
		}
		description, err = prompt(in, out, fmt.Sprintf("%s (optional): ", colorText(out, "1", "Description")))
		if err != nil {
			return err
		}
		if kind == packageCatalog {
			if err := editPackageSupplementalFields(supplemental, in, out); err != nil {
				return err
			}
		}
	}
	if name == "" {
		return fmt.Errorf("%s %s cannot be empty", catalogItemName(kind), catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	for _, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), name) {
			return fmt.Errorf("%s %q already exists in this project", catalogItemName(kind), name)
		}
	}
	localID := fmt.Sprintf("local-%s-%s", strings.TrimSuffix(string(kind), "s"), strconv.FormatInt(time.Now().UnixNano(), 36))
	record := map[string]any{
		"localId":              localID,
		"project":              map[string]any{"id": project.ID, "label": project.Name},
		catalogNameField(kind): name,
		"description":          description,
	}
	for field, value := range supplemental {
		record[field] = value
	}
	records = append(records, record)
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), name)
	return nil
}

func editCatalogRecordLocal(ws *workspace.Workspace, kind catalogKind, args []string, in io.Reader, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: panda %s edit <%s>", kind, catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	index := findCatalogRecord(records, kind, args[0])
	if index < 0 {
		return fmt.Errorf("%s %q was not found in the local project catalog; run `panda pull` first", catalogItemName(kind), args[0])
	}
	record := records[index]
	nameField := catalogNameField(kind)
	nameTitle := strings.ToUpper(nameField[:1]) + nameField[1:]
	name, err := prompt(in, out, fmt.Sprintf("%s [%s]: ", colorText(out, "1", nameTitle), catalogRecordName(kind, record)))
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) != "" {
		record[nameField] = strings.TrimSpace(name)
	}
	description, err := prompt(in, out, fmt.Sprintf("%s [Enter keeps current, - clears]: ", colorText(out, "1", "Description")))
	if err != nil {
		return err
	}
	if description == "-" {
		record["description"] = ""
	} else if strings.TrimSpace(description) != "" {
		record["description"] = description
	}
	if kind == packageCatalog {
		if err := editPackageSupplementalFields(record, in, out); err != nil {
			return err
		}
	}
	if err := ensureCatalogNameUnique(records, kind, index, catalogRecordName(kind, record)); err != nil {
		return err
	}
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), catalogRecordName(kind, record))
	return nil
}

func editPackageSupplementalFields(record map[string]any, in io.Reader, out io.Writer) error {
	startDate, err := prompt(in, out, fmt.Sprintf("%s [%s] (YYYY-MM-DD, - clears): ", colorText(out, "1", "Start date"), stringField(record, "startDate")))
	if err != nil {
		return err
	}
	if startDate == "-" {
		record["startDate"] = ""
	} else if strings.TrimSpace(startDate) != "" {
		record["startDate"] = strings.TrimSpace(startDate)
	}
	currentEstimation := ""
	if value := record["estimation"]; value != nil {
		currentEstimation = fmt.Sprint(value)
	}
	estimation, err := prompt(in, out, fmt.Sprintf("%s [%s] (- clears): ", colorText(out, "1", "Estimation"), currentEstimation))
	if err != nil {
		return err
	}
	if estimation == "-" {
		record["estimation"] = nil
	} else if strings.TrimSpace(estimation) != "" {
		value, err := strconv.ParseFloat(strings.TrimSpace(estimation), 64)
		if err != nil || value < 0 {
			return fmt.Errorf("invalid package estimation %q; enter a nonnegative number", estimation)
		}
		record["estimation"] = value
	}
	return nil
}

func deleteCatalogRecordLocal(ws *workspace.Workspace, kind catalogKind, args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: panda %s delete <%s>", kind, catalogNameField(kind))
	}
	records, err := readCatalog(ws, kind)
	if err != nil {
		return err
	}
	index := findCatalogRecord(records, kind, args[0])
	if index < 0 {
		return fmt.Errorf("%s %q was not found in the local project catalog", catalogItemName(kind), args[0])
	}
	deleted := records[index]
	identifier := catalogRecordID(deleted)
	records = append(records[:index], records[index+1:]...)
	if err := writeCatalog(ws, kind, records); err != nil {
		return err
	}
	if err := removeCatalogAssignments(ws, kind, identifier); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted local %s %q. Run `panda push` to publish it.\n", catalogItemName(kind), catalogRecordName(kind, deleted))
	return nil
}

func findCatalogRecord(records []map[string]any, kind catalogKind, selector string) int {
	for index, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), selector) || catalogRecordID(record) == selector {
			return index
		}
	}
	return -1
}

func ensureCatalogNameUnique(records []map[string]any, kind catalogKind, ignore int, name string) error {
	for index, record := range records {
		if index != ignore && strings.EqualFold(catalogRecordName(kind, record), name) {
			return fmt.Errorf("%s %q already exists in this project", catalogItemName(kind), name)
		}
	}
	return nil
}

func removeCatalogAssignments(ws *workspace.Workspace, kind catalogKind, identifier string) error {
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
		changed := false
		if kind == labelCatalog {
			labels, _ := metadata["labels"].([]any)
			filtered := make([]any, 0, len(labels))
			for _, value := range labels {
				relation, _ := value.(map[string]any)
				if relationshipID(relation) == identifier {
					changed = true
					continue
				}
				filtered = append(filtered, value)
			}
			if changed {
				metadata["labels"] = filtered
			}
		} else if relation, ok := metadata["package"].(map[string]any); ok && relationshipID(relation) == identifier {
			delete(metadata, "package")
			changed = true
		}
		if changed {
			if err := workspace.WriteMap(path, metadata); err != nil {
				return err
			}
		}
	}
	return nil
}

func editLabels(ws *workspace.Workspace, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	labels, err := readCatalog(ws, labelCatalog)
	if err != nil {
		return err
	}
	sort.Slice(labels, func(i, j int) bool {
		return strings.ToLower(catalogRecordName(labelCatalog, labels[i])) < strings.ToLower(catalogRecordName(labelCatalog, labels[j]))
	})
	if len(labels) == 0 {
		fmt.Fprintln(out, "No project labels. Create one with `panda labels create` or run `panda pull`.")
	}
	selected := make(map[string]map[string]any)
	if values, ok := metadata["labels"].([]any); ok {
		for _, value := range values {
			if relation, ok := value.(map[string]any); ok {
				if id := relationshipID(relation); id != "" {
					selected[id] = map[string]any{"id": id, "label": relationshipDisplayName(relation)}
				}
			}
		}
	}
	if len(requested) > 0 {
		if len(requested) == 1 && isNoneSelector(requested[0]) {
			selected = make(map[string]map[string]any)
		} else {
			for _, name := range requested {
				remove := strings.HasPrefix(name, "--remove=")
				name = strings.TrimPrefix(name, "--remove=")
				match := findCatalogRecordByName(labels, labelCatalog, name)
				if match == nil {
					return fmt.Errorf("label %q not found in the local project catalog; create it with `panda labels create` or run `panda pull`", name)
				}
				id := catalogRecordID(match)
				if remove {
					delete(selected, id)
				} else {
					selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, match)}
				}
			}
		}
	} else if isInteractiveTerminal(in, out) {
		pickerLabels := cloneCatalogRecords(labels)
		known := catalogNameIndex(labelCatalog, labels)
		for id, relation := range selected {
			if known[id] == "" {
				pickerLabels = append(pickerLabels, map[string]any{"id": id, "name": relationshipDisplayName(relation)})
			}
		}
		sort.Slice(pickerLabels, func(i, j int) bool {
			return strings.ToLower(catalogRecordName(labelCatalog, pickerLabels[i])) < strings.ToLower(catalogRecordName(labelCatalog, pickerLabels[j]))
		})
		choices := make([]selectorOption, 0, len(pickerLabels))
		for _, label := range pickerLabels {
			id := catalogRecordID(label)
			choices = append(choices, selectorOption{value: id, label: catalogRecordName(labelCatalog, label)})
		}
		selectedIDs := make(map[string]bool, len(selected))
		for id := range selected {
			selectedIDs[id] = true
		}
		selectedIDs, err = selectMultipleOptions(in, out, "Labels", choices, selectedIDs)
		if err != nil {
			return err
		}
		selected = make(map[string]map[string]any, len(selectedIDs))
		for _, label := range pickerLabels {
			id := catalogRecordID(label)
			if selectedIDs[id] {
				selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, label)}
			}
		}
	} else {
		finished := false
		for !finished {
			prefix, err := prompt(in, out, fmt.Sprintf("Search %s by prefix (blank to finish, `none` to clear all): ", colorText(out, "1", "labels")))
			if err != nil {
				return err
			}
			if strings.EqualFold(prefix, "none") {
				selected = make(map[string]map[string]any)
				break
			}
			if prefix == "" {
				break
			}
			for {
				matches := make([]map[string]any, 0)
				for _, label := range labels {
					if strings.HasPrefix(strings.ToLower(catalogRecordName(labelCatalog, label)), strings.ToLower(prefix)) {
						matches = append(matches, label)
					}
				}
				if len(matches) == 0 {
					fmt.Fprintln(out, "No labels match that prefix.")
					break
				}
				for index, label := range matches {
					id := catalogRecordID(label)
					mark := " "
					if _, exists := selected[id]; exists {
						mark = "✓"
					}
					fmt.Fprintf(out, "  %s %d) %s\n", mark, index+1, catalogRecordName(labelCatalog, label))
				}
				choice, err := prompt(in, out, "Select a number to toggle, type a narrower prefix, or press Enter to finish: ")
				if err != nil {
					return err
				}
				if choice == "" {
					finished = true
					break
				}
				index, parseErr := strconv.Atoi(choice)
				if parseErr != nil || index < 1 || index > len(matches) {
					prefix = choice
					continue
				}
				label := matches[index-1]
				id := catalogRecordID(label)
				if _, exists := selected[id]; exists {
					delete(selected, id)
				} else {
					selected[id] = map[string]any{"id": id, "label": catalogRecordName(labelCatalog, label)}
				}
				break
			}
		}
	}
	result := make([]any, 0, len(selected))
	for _, relation := range selected {
		result = append(result, relation)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := result[i].(map[string]any)
		right, _ := result[j].(map[string]any)
		return stringField(left, "label") < stringField(right, "label")
	})
	metadata["labels"] = result
	return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
}

func editTaskPackage(ws *workspace.Workspace, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	if stringField(metadata, "type") == "release" {
		if len(requested) == 1 && isNoneSelector(requested[0]) {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		return errors.New("release tasks cannot be assigned to a package")
	}
	packages, err := readCatalog(ws, packageCatalog)
	if err != nil {
		return err
	}
	sort.Slice(packages, func(i, j int) bool {
		return strings.ToLower(catalogRecordName(packageCatalog, packages[i])) < strings.ToLower(catalogRecordName(packageCatalog, packages[j]))
	})
	if len(requested) > 0 {
		selection := strings.TrimSpace(strings.Join(requested, " "))
		if isNoneSelector(selection) {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		match := findCatalogRecordByName(packages, packageCatalog, selection)
		if match == nil {
			return fmt.Errorf("package %q not found in the local project catalog; create it with `panda packages create` or run `panda pull`", selection)
		}
		metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	if isInteractiveTerminal(in, out) {
		choices := make([]selectorOption, 0, len(packages)+1)
		choices = append(choices, selectorOption{value: "", label: "None (no package)"})
		currentID := relationshipID(metadata["package"])
		currentInCatalog := false
		for _, pkg := range packages {
			id := catalogRecordID(pkg)
			choices = append(choices, selectorOption{value: id, label: catalogRecordName(packageCatalog, pkg)})
			currentInCatalog = currentInCatalog || id == currentID
		}
		if currentID != "" && !currentInCatalog {
			choices = append(choices, selectorOption{value: currentID, label: relationshipDisplayName(metadata["package"]) + " (current)"})
		}
		selected, err := selectSingleOption(in, out, "Package", choices, currentID)
		if err != nil {
			return err
		}
		if selected == "" {
			delete(metadata, "package")
			return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
		}
		match := findCatalogRecordByName(packages, packageCatalog, selected)
		if match == nil {
			if selected == currentID {
				return nil
			}
			return fmt.Errorf("package %q is not in the local project catalog", selected)
		}
		metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	current := relationshipDisplayName(metadata["package"])
	if len(packages) == 0 {
		fmt.Fprintln(out, "No project packages. Create one with `panda packages create` or run `panda pull`.")
		return nil
	}
	fmt.Fprintln(out, "Available packages:")
	for index, pkg := range packages {
		fmt.Fprintf(out, "  %d) %s\n", index+1, catalogRecordName(packageCatalog, pkg))
	}
	choice, err := prompt(in, out, fmt.Sprintf("%s [%s] (number or name, `none` to clear, Enter to keep): ", colorText(out, "1", "Package"), current))
	if err != nil {
		return err
	}
	if choice == "" {
		return nil
	}
	if isNoneSelector(choice) {
		delete(metadata, "package")
		return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
	}
	var match map[string]any
	if index, parseErr := strconv.Atoi(choice); parseErr == nil && index >= 1 && index <= len(packages) {
		match = packages[index-1]
	} else {
		match = findCatalogRecordByName(packages, packageCatalog, choice)
	}
	if match == nil {
		return fmt.Errorf("package %q not found in the local project catalog", choice)
	}
	metadata["package"] = map[string]any{"id": catalogRecordID(match), "label": catalogRecordName(packageCatalog, match)}
	return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
}

func findCatalogRecordByName(records []map[string]any, kind catalogKind, name string) map[string]any {
	for _, record := range records {
		if strings.EqualFold(catalogRecordName(kind, record), strings.TrimSpace(name)) || catalogRecordID(record) == name {
			return record
		}
	}
	return nil
}

func isNoneSelector(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "none") || strings.TrimSpace(value) == "-"
}

func hasPackageAssignment(value any) bool {
	if relation, ok := value.(map[string]any); ok {
		return relationshipID(relation) != "" || relationshipDisplayName(relation) != ""
	}
	return !isEmptyField(value)
}

func hasLabelAssignments(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}

func relationshipDisplayName(value any) string {
	relation, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, field := range []string{"label", "name", "title", "id"} {
		if name := strings.TrimSpace(stringField(relation, field)); name != "" {
			return name
		}
	}
	return ""
}

func catalogNameIndex(kind catalogKind, records []map[string]any) map[string]string {
	index := make(map[string]string, len(records))
	for _, record := range records {
		id := catalogRecordID(record)
		if id != "" {
			index[id] = catalogRecordName(kind, record)
		}
	}
	return index
}

func relationshipDisplayNameWithCatalog(value any, catalogNames map[string]string) string {
	if id := relationshipID(value); id != "" {
		if name := catalogNames[id]; name != "" {
			return name
		}
	}
	return relationshipDisplayName(value)
}

func labelsDisplay(value any) string {
	return labelsDisplayWithCatalog(value, nil)
}

func labelsDisplayWithCatalog(value any, catalogNames map[string]string) string {
	items, _ := value.([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := relationshipDisplayNameWithCatalog(item, catalogNames); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return strings.Join(names, ", ")
}

func remapTaskCatalogRelationships(task map[string]any, idMap map[string]string) (map[string]any, error) {
	result := cloneMap(task)
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

func rewriteTaskCatalogIDs(ws *workspace.Workspace, idMap map[string]string) error {
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
		updated := cloneMap(metadata)
		changed := false
		if labels, ok := updated["labels"].([]any); ok {
			mapped := make([]any, 0, len(labels))
			for _, value := range labels {
				relation, ok := value.(map[string]any)
				if ok {
					if remoteID, exists := idMap[relationshipID(relation)]; exists {
						relation = cloneMap(relation)
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
			if remoteID, exists := idMap[relationshipID(relation)]; exists {
				relation = cloneMap(relation)
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
	updated := cloneMap(relation)
	id := relationshipID(updated)
	if remoteID, exists := idMap[id]; exists {
		updated["id"] = remoteID
		return updated, nil
	}
	if strings.HasPrefix(id, "local-label-") || strings.HasPrefix(id, "local-package-") {
		return nil, fmt.Errorf("catalog item %q has not been pushed yet", relationshipDisplayName(updated))
	}
	return updated, nil
}
