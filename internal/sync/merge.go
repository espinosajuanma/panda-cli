package sync

import (
	"os"
	"path/filepath"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/task"
)

func MergeForPush(local, remote, baseline map[string]any, dir string) (map[string]any, bool, error) {
	merged, changed := task.MergeFields(task.FromMap(local), task.FromMap(remote), task.FromMap(baseline))
	mergedMap := merged.Map()
	baseMarkdown, err := content.HTMLToMarkdown(jsonutil.StringField(baseline, "description"))
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

func TaskConflicts(local, baseline, remote map[string]any, dir string) []string {
	conflicts := task.MetadataConflicts(task.FromMap(local), task.FromMap(baseline), task.FromMap(remote))
	baseMarkdown, err := content.HTMLToMarkdown(jsonutil.StringField(baseline, "description"))
	if err != nil {
		return append(conflicts, "description")
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return append(conflicts, "description")
	}
	if remoteHTML := jsonutil.StringField(remote, "description"); remoteHTML != jsonutil.StringField(baseline, "description") {
		remoteMarkdown, convertErr := content.HTMLToMarkdown(remoteHTML)
		if convertErr != nil || (string(localMarkdown) != baseMarkdown && string(localMarkdown) != remoteMarkdown) {
			conflicts = append(conflicts, "description")
		}
	}
	noteConflicts, err := LocalNoteConflicts(dir, baseline, remote)
	if err != nil {
		conflicts = append(conflicts, "notes")
	} else {
		for _, filename := range noteConflicts {
			conflicts = append(conflicts, filepath.ToSlash(filepath.Join("notes", filename)))
		}
	}
	releaseConflicts, err := ReleaseNoteConflicts(dir, baseline, remote)
	if err != nil {
		conflicts = append(conflicts, ReleaseCustomerNotesFile, ReleaseInternalNotesFile)
	} else {
		conflicts = append(conflicts, releaseConflicts...)
	}
	return conflicts
}

func PullMetadataConflicts(local, baseline, remote map[string]any) []string {
	return task.MetadataConflicts(task.FromMap(local), task.FromMap(baseline), task.FromMap(remote))
}

func AdvanceBaseline(baseline, remote map[string]any, conflicts []string, descriptionConflict bool) map[string]any {
	return task.AdvanceBaseline(task.FromMap(baseline), task.FromMap(remote), conflicts, descriptionConflict).Map()
}
