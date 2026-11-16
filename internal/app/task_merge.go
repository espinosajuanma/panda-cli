package app

import (
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
)

func mergeForPush(local, remote, baseline map[string]any, dir string) (map[string]any, bool, error) {
	return syncpkg.MergeForPush(local, remote, baseline, dir)
}

func taskConflicts(local, baseline, remote map[string]any, dir string) []string {
	return syncpkg.TaskConflicts(local, baseline, remote, dir)
}

func pullMetadataConflicts(local, baseline, remote map[string]any) []string {
	return syncpkg.PullMetadataConflicts(local, baseline, remote)
}

func advanceBaseline(baseline, remote map[string]any, conflicts []string, descriptionConflict bool) map[string]any {
	return syncpkg.AdvanceBaseline(baseline, remote, conflicts, descriptionConflict)
}
