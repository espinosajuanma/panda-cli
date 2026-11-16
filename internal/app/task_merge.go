package app

import (
	"os"
	"path/filepath"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/task"
)

func mergeForPush(local, remote, baseline map[string]any, dir string) (map[string]any, bool, error) {
	merged, changed := task.MergeFields(task.FromMap(local), task.FromMap(remote), task.FromMap(baseline))
	mergedMap := merged.Map()
	baseMarkdown, err := content.HTMLToMarkdown(stringField(baseline, "description"))
	if err != nil {
		return nil, false, err
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return nil, false, err
	}
	if string(localMarkdown) != baseMarkdown {
		html, err := content.MarkdownToHTML(string(localMarkdown))
		if err != nil {
			return nil, false, err
		}
		mergedMap["description"] = html
		changed = true
	}
	return mergedMap, changed, nil
}

func taskConflicts(local, baseline, remote map[string]any, dir string) []string {
	conflicts := task.MetadataConflicts(task.FromMap(local), task.FromMap(baseline), task.FromMap(remote))
	baseMarkdown, err := content.HTMLToMarkdown(stringField(baseline, "description"))
	if err != nil {
		return append(conflicts, "description")
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return append(conflicts, "description")
	}
	if remoteHTML := stringField(remote, "description"); remoteHTML != stringField(baseline, "description") {
		remoteMarkdown, convertErr := content.HTMLToMarkdown(remoteHTML)
		if convertErr != nil || (string(localMarkdown) != baseMarkdown && string(localMarkdown) != remoteMarkdown) {
			conflicts = append(conflicts, "description")
		}
	}
	noteConflicts, err := localNoteConflicts(dir, baseline, remote)
	if err != nil {
		conflicts = append(conflicts, "notes")
	} else {
		for _, filename := range noteConflicts {
			conflicts = append(conflicts, filepath.ToSlash(filepath.Join("notes", filename)))
		}
	}
	releaseConflicts, err := releaseNoteConflicts(dir, baseline, remote)
	if err != nil {
		conflicts = append(conflicts, releaseCustomerNotesFile, releaseInternalNotesFile)
	} else {
		conflicts = append(conflicts, releaseConflicts...)
	}
	return conflicts
}

func pullMetadataConflicts(local, baseline, remote map[string]any) []string {
	return task.MetadataConflicts(task.FromMap(local), task.FromMap(baseline), task.FromMap(remote))
}

func advanceBaseline(baseline, remote map[string]any, conflicts []string, descriptionConflict bool) map[string]any {
	return task.AdvanceBaseline(task.FromMap(baseline), task.FromMap(remote), conflicts, descriptionConflict).Map()
}
