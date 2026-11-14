package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

const (
	releaseCustomerNotesFile = "customer-notes.md"
	releaseInternalNotesFile = "internal-notes.md"
)

type releaseNoteField struct {
	name     string
	filename string
}

var releaseNoteFields = []releaseNoteField{
	{name: "customerNotes", filename: releaseCustomerNotesFile},
	{name: "internalNotes", filename: releaseInternalNotesFile},
}

type localReleaseNotes struct {
	markdown map[string]string
	exists   map[string]bool
}

func releaseNotesObject(remote map[string]any) map[string]any {
	if remote == nil {
		return nil
	}
	notes, _ := remote["releaseNotes"].(map[string]any)
	return notes
}

func releaseNoteHTML(remote map[string]any, field string) string {
	notes := releaseNotesObject(remote)
	if notes == nil {
		return ""
	}
	return stringField(notes, field)
}

func releaseNoteMarkdown(remote map[string]any, field string) (string, error) {
	return content.HTMLToMarkdown(releaseNoteHTML(remote, field))
}

func releaseNoteMarkdownValues(remote map[string]any) (map[string]string, error) {
	values := make(map[string]string, len(releaseNoteFields))
	for _, field := range releaseNoteFields {
		markdown, err := releaseNoteMarkdown(remote, field.name)
		if err != nil {
			return nil, err
		}
		values[field.name] = markdown
	}
	return values, nil
}

func writeReleaseNotesToLocal(dir string, remote map[string]any) error {
	values, err := releaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	for _, field := range releaseNoteFields {
		if err := workspace.WriteFileAtomic(filepath.Join(dir, field.filename), []byte(values[field.name]), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func ensureReleaseNotesFiles(dir string, remote map[string]any) error {
	values, err := releaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	for _, field := range releaseNoteFields {
		path := filepath.Join(dir, field.filename)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := workspace.WriteFileAtomic(path, []byte(values[field.name]), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func readLocalReleaseNotes(dir string) (localReleaseNotes, error) {
	result := localReleaseNotes{
		markdown: make(map[string]string, len(releaseNoteFields)),
		exists:   make(map[string]bool, len(releaseNoteFields)),
	}
	for _, field := range releaseNoteFields {
		data, err := os.ReadFile(filepath.Join(dir, field.filename))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return localReleaseNotes{}, err
		}
		result.markdown[field.name] = string(data)
		result.exists[field.name] = true
	}
	return result, nil
}

func releaseNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	local, err := readLocalReleaseNotes(dir)
	if err != nil {
		return nil, err
	}
	base, err := releaseNoteMarkdownValues(baseline)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for _, field := range releaseNoteFields {
		if !local.exists[field.name] {
			continue
		}
		if local.markdown[field.name] != base[field.name] {
			changed[field.name] = local.markdown[field.name]
		}
	}
	return changed, nil
}

func releaseNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	changes, err := releaseNoteChanges(dir, baseline)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	base, err := releaseNoteMarkdownValues(baseline)
	if err != nil {
		return nil, err
	}
	current, err := releaseNoteMarkdownValues(remote)
	if err != nil {
		return nil, err
	}
	var conflicts []string
	for _, field := range releaseNoteFields {
		localMarkdown, changed := changes[field.name]
		if !changed {
			continue
		}
		if current[field.name] != base[field.name] && current[field.name] != localMarkdown {
			conflicts = append(conflicts, field.filename)
		}
	}
	return conflicts, nil
}

func releaseNotesPayload(dir string, baseline map[string]any) (map[string]string, error) {
	changes, err := releaseNoteChanges(dir, baseline)
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

func syncReleaseNotesFromPull(dir string, baseline, remote map[string]any, conflicts []string) error {
	base, err := releaseNoteMarkdownValues(baseline)
	if err != nil {
		return err
	}
	current, err := releaseNoteMarkdownValues(remote)
	if err != nil {
		return err
	}
	local, err := readLocalReleaseNotes(dir)
	if err != nil {
		return err
	}
	for _, field := range releaseNoteFields {
		if contains(conflicts, field.filename) {
			continue
		}
		localChanged := local.exists[field.name] && local.markdown[field.name] != base[field.name]
		if localChanged && current[field.name] == base[field.name] {
			continue
		}
		if err := workspace.WriteFileAtomic(filepath.Join(dir, field.filename), []byte(current[field.name]), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func advanceReleaseNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	if len(conflicts) == 0 {
		return remote
	}
	baseNotes := releaseNotesObject(baseline)
	if baseNotes == nil {
		return remote
	}
	advanced := cloneMap(remote)
	current, _ := advanced["releaseNotes"].(map[string]any)
	if current == nil {
		current = make(map[string]any)
		advanced["releaseNotes"] = current
	}
	for _, field := range releaseNoteFields {
		if !contains(conflicts, field.filename) {
			continue
		}
		if value, exists := baseNotes[field.name]; exists {
			current[field.name] = value
		} else {
			delete(current, field.name)
		}
	}
	return advanced
}

func editReleaseNotesFiles(dir string, args []string, out io.Writer) error {
	selected := releaseNoteFields
	if len(args) > 1 {
		return errors.New("usage: panda edit <taskNumber|draft-N> release-notes [customer|internal]")
	}
	if len(args) == 1 {
		switch args[0] {
		case "customer":
			selected = releaseNoteFields[:1]
		case "internal":
			selected = releaseNoteFields[1:]
		default:
			return errors.New("usage: panda edit <taskNumber|draft-N> release-notes [customer|internal]")
		}
	}
	for _, field := range selected {
		path := filepath.Join(dir, field.filename)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := workspace.WriteFileAtomic(path, nil, 0o600); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := openEditor(path); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Edited release notes. Review with `panda diff --verbose`; run `panda push` to apply them.\n")
	return nil
}

func releaseNoteDisplayName(field string) string {
	switch field {
	case "customerNotes":
		return "customer notes"
	case "internalNotes":
		return "internal notes"
	default:
		return strings.TrimSpace(field)
	}
}

func releaseNoteHeading(field string) string {
	name := releaseNoteDisplayName(field)
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
