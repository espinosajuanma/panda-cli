package sync

import (
	"context"
	"fmt"
	"io"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func PullProjectCatalogs(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) (int, error) {
	remote := make(map[catdomain.Kind][]map[string]any, 2)
	for _, kind := range []catdomain.Kind{catdomain.Labels, catdomain.Packages} {
		records, err := CatalogRecords(ctx, client, project.ID, kind)
		if err != nil {
			return 0, fmt.Errorf("pull project %s: %w", kind, err)
		}
		remote[kind] = records
	}
	return PullProjectCatalogsFromRecords(ws, remote, out, nil)
}

func PullProjectCatalogsFromRecords(ws *workspace.Workspace, records map[catdomain.Kind][]map[string]any, out io.Writer, progress *PullProgress) (int, error) {
	conflicts := 0
	counts := make(map[catdomain.Kind]int, 2)
	var reports []string
	for index, kind := range []catdomain.Kind{catdomain.Labels, catdomain.Packages} {
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
	fmt.Fprintf(out, "Synchronized project catalogs: %d labels, %d packages.\n", counts[catdomain.Labels], counts[catdomain.Packages])
	return conflicts, nil
}

func pullProjectCatalogKind(ws *workspace.Workspace, kind catdomain.Kind, remoteRecords []map[string]any, out io.Writer, progress *PullProgress, start, end int, reports *[]string) (int, int, error) {
	remoteRecords = catdomain.DedupeRecords(remoteRecords)
	remote, err := catdomain.NormalizeRecords(kind, remoteRecords)
	if err != nil {
		return 0, 0, err
	}
	baseline, err := ws.ReadCatalogBaseline(kind)
	if err != nil {
		return 0, 0, err
	}
	local, err := ReadLocalCatalogForSync(ws, kind, baseline)
	if err != nil {
		return 0, 0, err
	}
	merged, advanced, issues := catdomain.MergeRecords(kind, local, baseline, remote)
	for _, issue := range issues {
		*reports = append(*reports, fmt.Sprintf("%s project %s: %s", ui.ColorText(out, "1;31", "Conflict"), kind, issue))
	}
	if err := ws.WriteCatalog(kind, merged); err != nil {
		return len(issues), 0, err
	}
	if err := ws.WriteCatalogBaseline(kind, advanced); err != nil {
		return len(issues), 0, err
	}
	if progress != nil {
		for index := range remote {
			UpdatePullStage(progress, start, end, index+1, len(remote))
		}
		if len(remote) == 0 {
			progress.Update(end)
		}
	}
	return len(issues), len(merged), nil
}

func DedupeCatalogRecords(records []map[string]any) []map[string]any {
	return catdomain.DedupeRecords(records)
}

func CatalogDiff(ws *workspace.Workspace) ([]CatalogChange, int, int, error) {
	var changes []CatalogChange
	labelChanges, packageChanges := 0, 0
	for _, kind := range []catdomain.Kind{catdomain.Labels, catdomain.Packages} {
		local, err := ws.ReadCatalog(kind)
		if err != nil {
			return nil, 0, 0, err
		}
		baseline, err := ws.ReadCatalogBaseline(kind)
		if err != nil {
			return nil, 0, 0, err
		}
		baseByID := make(map[string]map[string]any, len(baseline))
		localByID := make(map[string]map[string]any, len(local))
		for _, record := range baseline {
			baseByID[jsonutil.StringField(record, "id")] = record
		}
		for _, record := range local {
			if id := jsonutil.StringField(record, "id"); id != "" {
				localByID[id] = record
			}
		}
		for _, record := range local {
			name := catdomain.RecordName(kind, record)
			if jsonutil.StringField(record, "localId") != "" {
				changes = append(changes, CatalogChange{Kind: string(kind), Action: "new", Name: name})
				if kind == catdomain.Labels {
					labelChanges++
				} else {
					packageChanges++
				}
				continue
			}
			base := baseByID[jsonutil.StringField(record, "id")]
			if base == nil {
				changes = append(changes, CatalogChange{Kind: string(kind), Action: "new", Name: name})
				if kind == catdomain.Labels {
					labelChanges++
				} else {
					packageChanges++
				}
				continue
			}
			var changed []string
			for _, field := range catdomain.Fields(kind) {
				if !jsonutil.Same(record[field], base[field]) {
					changed = append(changed, field)
				}
			}
			if len(changed) > 0 {
				changes = append(changes, CatalogChange{Kind: string(kind), Action: "edited", Name: name, Fields: changed})
				if kind == catdomain.Labels {
					labelChanges++
				} else {
					packageChanges++
				}
			}
		}
		for _, record := range baseline {
			id := jsonutil.StringField(record, "id")
			if id != "" && localByID[id] == nil {
				changes = append(changes, CatalogChange{Kind: string(kind), Action: "deleted", Name: catdomain.RecordName(kind, record)})
				if kind == catdomain.Labels {
					labelChanges++
				} else {
					packageChanges++
				}
			}
		}
	}
	return changes, labelChanges, packageChanges, nil
}

type CatalogChange struct {
	Kind   string
	Action string
	Name   string
	Fields []string
}
