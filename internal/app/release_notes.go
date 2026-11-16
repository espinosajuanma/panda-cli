package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

const (
	releaseCustomerNotesFile = syncpkg.ReleaseCustomerNotesFile
	releaseInternalNotesFile = syncpkg.ReleaseInternalNotesFile
)

type releaseNoteField = syncpkg.ReleaseNoteField

type localReleaseNotes = syncpkg.LocalReleaseNotes

var releaseNoteFields = syncpkg.ReleaseNoteFields

func releaseNotesObject(remote map[string]any) map[string]any {
	return syncpkg.ReleaseNotesObject(remote)
}

func releaseNoteHTML(remote map[string]any, field string) string {
	return syncpkg.ReleaseNoteHTML(remote, field)
}

func releaseNoteMarkdown(remote map[string]any, field string) (string, error) {
	return syncpkg.ReleaseNoteMarkdown(remote, field)
}

func releaseNoteMarkdownValues(remote map[string]any) (map[string]string, error) {
	return syncpkg.ReleaseNoteMarkdownValues(remote)
}

func writeReleaseNotesToLocal(dir string, remote map[string]any) error {
	return syncpkg.WriteReleaseNotesToLocal(dir, remote)
}

func ensureReleaseNotesFiles(dir string, remote map[string]any) error {
	return syncpkg.EnsureReleaseNotesFiles(dir, remote)
}

func readLocalReleaseNotes(dir string) (localReleaseNotes, error) {
	return syncpkg.ReadLocalReleaseNotes(dir)
}

func releaseNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	return syncpkg.ReleaseNoteChanges(dir, baseline)
}

func releaseNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	return syncpkg.ReleaseNoteConflicts(dir, baseline, remote)
}

func releaseNotesPayload(dir string, baseline map[string]any) (map[string]string, error) {
	return syncpkg.ReleaseNotesPayload(dir, baseline)
}

func syncReleaseNotesFromPull(dir string, baseline, remote map[string]any, conflicts []string) error {
	return syncpkg.SyncReleaseNotesFromPull(dir, baseline, remote, conflicts)
}

func advanceReleaseNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	return syncpkg.AdvanceReleaseNotesBaseline(baseline, remote, conflicts)
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
		path := filepath.Join(dir, field.Filename)
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
	return report.ReleaseNoteDisplayName(field)
}

func releaseNoteHeading(field string) string {
	name := releaseNoteDisplayName(field)
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
