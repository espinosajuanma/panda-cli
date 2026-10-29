package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

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
	remote := make(map[catalogKind][]map[string]any, 2)
	for _, kind := range []catalogKind{labelCatalog, packageCatalog} {
		records, err := catalogRecords(ctx, client, project.ID, kind)
		if err != nil {
			return 0, fmt.Errorf("pull project %s: %w", kind, err)
		}
		remote[kind] = records
	}
	return pullProjectCatalogsFromRecords(ws, remote, out, nil)
}

func pullProjectCatalogsFromRecords(ws *workspace.Workspace, records map[catalogKind][]map[string]any, out io.Writer, progress *pullProgress) (int, error) {
	conflicts := 0
	counts := make(map[catalogKind]int, 2)
	var reports []string
	for index, kind := range []catalogKind{labelCatalog, packageCatalog} {
		start, end := index*50, (index+1)*50
		kindConflicts, count, err := pullProjectCatalogKind(ws, kind, records[kind], out, progress, start, end, &reports)
		conflicts += kindConflicts
		if err != nil {
			return conflicts, err
		}
		counts[kind] = count
	}
	if progress != nil {
		progress.Finish()
	}
	for _, report := range reports {
		fmt.Fprintln(out, report)
	}
	fmt.Fprintf(out, "Synchronized project catalogs: %d labels, %d packages.\n", counts[labelCatalog], counts[packageCatalog])
	return conflicts, nil
}

func pullProjectCatalogKind(ws *workspace.Workspace, kind catalogKind, remoteRecords []map[string]any, out io.Writer, progress *pullProgress, start, end int, reports *[]string) (int, int, error) {
	remoteRecords = dedupeCatalogRecords(remoteRecords)
	remote, err := normalizeCatalogRecords(kind, remoteRecords)
	if err != nil {
		return 0, 0, err
	}
	baseline, err := readCatalogBaseline(ws, kind)
	if err != nil {
		return 0, 0, err
	}
	local, err := readLocalCatalogForSync(ws, kind, baseline)
	if err != nil {
		return 0, 0, err
	}
	merged, advanced, issues := mergeCatalogRecords(kind, local, baseline, remote)
	for _, issue := range issues {
		*reports = append(*reports, fmt.Sprintf("%s project %s: %s", colorText(out, "1;31", "Conflict"), kind, issue))
	}
	if err := writeCatalog(ws, kind, merged); err != nil {
		return len(issues), 0, err
	}
	if err := writeCatalogBaseline(ws, kind, advanced); err != nil {
		return len(issues), 0, err
	}
	if progress != nil {
		for index := range remote {
			updatePullStage(progress, start, end, index+1, len(remote))
		}
		if len(remote) == 0 {
			progress.Update(end)
		}
	}
	return len(issues), len(merged), nil
}

func dedupeCatalogRecords(records []map[string]any) []map[string]any {
	return catdomain.DedupeRecords(records)
}
