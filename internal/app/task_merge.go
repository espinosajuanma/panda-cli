package app

import (
	"os"
	"path/filepath"

	"panda-dashboard/internal/content"
)

func mergeForPush(local, remote, baseline map[string]any, dir string) (map[string]any, bool, error) {
	merged := cloneMap(remote)
	changed := false
	for _, field := range editableFields {
		localValue := valueAt(local, field)
		baseValue := valueAt(baseline, field)
		remoteValue := valueAt(remote, field)
		if !same(localValue, baseValue) && !same(localValue, remoteValue) {
			merged[field] = localValue
			changed = true
		}
	}
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
		merged["description"] = html
		changed = true
	}
	return merged, changed, nil
}

func taskConflicts(local, baseline, remote map[string]any, dir string) []string {
	var conflicts []string
	for _, field := range editableFields {
		localValue, baseValue, remoteValue := valueAt(local, field), valueAt(baseline, field), valueAt(remote, field)
		if !same(localValue, baseValue) && !same(remoteValue, baseValue) && !same(localValue, remoteValue) {
			conflicts = append(conflicts, field)
		}
	}
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
	return conflicts
}

func pullMetadataConflicts(local, baseline, remote map[string]any) []string {
	var conflicts []string
	for _, field := range editableFields {
		localValue, baseValue, remoteValue := valueAt(local, field), valueAt(baseline, field), valueAt(remote, field)
		if !same(localValue, baseValue) && !same(remoteValue, baseValue) && !same(localValue, remoteValue) {
			conflicts = append(conflicts, field)
		}
	}
	return conflicts
}

func advanceBaseline(baseline, remote map[string]any, conflicts []string, descriptionConflict bool) map[string]any {
	advanced := cloneMap(remote)
	for _, field := range conflicts {
		if value, exists := baseline[field]; exists {
			advanced[field] = value
		} else {
			delete(advanced, field)
		}
	}
	if descriptionConflict {
		advanced["description"] = stringField(baseline, "description")
	}
	return advanced
}
