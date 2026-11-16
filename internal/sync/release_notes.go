package sync

import (
	"fmt"
	"slices"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

const (
	ReleaseCustomerNotesFile = "customer-notes.md"
	ReleaseInternalNotesFile = "internal-notes.md"
)

type ReleaseNoteField struct {
	Name     string
	Filename string
}

var ReleaseNoteFields = []ReleaseNoteField{
	{Name: "customerNotes", Filename: ReleaseCustomerNotesFile},
	{Name: "internalNotes", Filename: ReleaseInternalNotesFile},
}

type LocalReleaseNotes struct {
	Markdown map[string]string
	Exists   map[string]bool
}

func ReleaseNotesObject(remote map[string]any) map[string]any {
	if remote == nil {
		return nil
	}
	notes, _ := remote["releaseNotes"].(map[string]any)
	return notes
}

func ReleaseNoteHTML(remote map[string]any, field string) string {
	notes := ReleaseNotesObject(remote)
	if notes == nil {
		return ""
	}
	return jsonutil.StringField(notes, field)
}

func ReleaseNoteMarkdown(remote map[string]any, field string) (string, error) {
	return content.HTMLToMarkdown(ReleaseNoteHTML(remote, field))
}

func ReleaseNoteMarkdownValues(remote map[string]any) (map[string]string, error) {
	values := make(map[string]string, len(ReleaseNoteFields))
	for _, field := range ReleaseNoteFields {
		markdown, err := ReleaseNoteMarkdown(remote, field.Name)
		if err != nil {
			return nil, err
		}
		values[field.Name] = markdown
	}
	return values, nil
}

func WriteReleaseNotesToLocal(dir string, remote map[string]any) error {
	values, err := ReleaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	for _, field := range ReleaseNoteFields {
		if err := workspace.WriteTextFile(dir, field.Filename, values[field.Name]); err != nil {
			return err
		}
	}
	return nil
}

func EnsureReleaseNotesFiles(dir string, remote map[string]any) error {
	values, err := ReleaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	for _, field := range ReleaseNoteFields {
		if err := workspace.EnsureTextFile(dir, field.Filename, values[field.Name]); err != nil {
			return err
		}
	}
	return nil
}

func ReadLocalReleaseNotes(dir string) (LocalReleaseNotes, error) {
	result := LocalReleaseNotes{
		Markdown: make(map[string]string, len(ReleaseNoteFields)),
		Exists:   make(map[string]bool, len(ReleaseNoteFields)),
	}
	for _, field := range ReleaseNoteFields {
		data, exists, err := workspace.ReadTextFile(dir, field.Filename)
		if err != nil {
			return LocalReleaseNotes{}, err
		}
		if !exists {
			continue
		}
		result.Markdown[field.Name] = data
		result.Exists[field.Name] = true
	}
	return result, nil
}

func ReleaseNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	local, err := ReadLocalReleaseNotes(dir)
	if err != nil {
		return nil, err
	}
	base, err := ReleaseNoteMarkdownValues(baseline)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for _, field := range ReleaseNoteFields {
		if !local.Exists[field.Name] {
			continue
		}
		if local.Markdown[field.Name] != base[field.Name] {
			changed[field.Name] = local.Markdown[field.Name]
		}
	}
	return changed, nil
}

func ReleaseNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	changes, err := ReleaseNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	base, err := ReleaseNoteMarkdownValues(baseline)
	if err != nil {
		return nil, err
	}
	current, err := ReleaseNoteMarkdownValues(remote)
	if err != nil {
		return nil, err
	}
	var conflicts []string
	for _, field := range ReleaseNoteFields {
		localMarkdown, changed := changes[field.Name]
		if !changed {
			continue
		}
		if current[field.Name] != base[field.Name] && current[field.Name] != localMarkdown {
			conflicts = append(conflicts, field.Filename)
		}
	}
	return conflicts, nil
}

func ReleaseNotesPayload(dir string, baseline map[string]any) (map[string]string, error) {
	changes, err := ReleaseNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	payload := make(map[string]string, len(changes))
	for field, markdown := range changes {
		html, err := content.MarkdownToHTML(markdown)
		if err != nil {
			return nil, fmt.Errorf("convert %s: %w", field, err)
		}
		payload[field] = html
	}
	return payload, nil
}

func SyncReleaseNotesFromPull(dir string, baseline, remote map[string]any, conflicts []string) error {
	base, err := ReleaseNoteMarkdownValues(baseline)
	if err != nil {
		return err
	}
	current, err := ReleaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	local, err := ReadLocalReleaseNotes(dir)
	if err != nil {
		return err
	}
	for _, field := range ReleaseNoteFields {
		if slices.Contains(conflicts, field.Filename) {
			continue
		}
		localChanged := local.Exists[field.Name] && local.Markdown[field.Name] != base[field.Name]
		if localChanged && current[field.Name] == base[field.Name] {
			continue
		}
		if err := workspace.WriteTextFile(dir, field.Filename, current[field.Name]); err != nil {
			return err
		}
	}
	return nil
}

func AdvanceReleaseNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	if len(conflicts) == 0 {
		return remote
	}
	baseNotes := ReleaseNotesObject(baseline)
	if baseNotes == nil {
		return remote
	}
	advanced := jsonutil.CloneMap(remote)
	current, _ := advanced["releaseNotes"].(map[string]any)
	if current == nil {
		current = make(map[string]any)
		advanced["releaseNotes"] = current
	}
	for _, field := range ReleaseNoteFields {
		if !slices.Contains(conflicts, field.Filename) {
			continue
		}
		if value, exists := baseNotes[field.Name]; exists {
			current[field.Name] = value
		} else {
			delete(current, field.Name)
		}
	}
	return advanced
}
