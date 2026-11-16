package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsLocalDraftRecognizesDraftAndLegacyIDs(t *testing.T) {
	tests := []struct {
		dir      string
		metadata map[string]any
		want     bool
	}{
		{dir: "/tasks/42", metadata: map[string]any{"id": "task-42"}, want: false},
		{dir: "/tasks/draft-3", metadata: map[string]any{"localId": "draft-3"}, want: true},
		{dir: "/tasks/draft-3", metadata: map[string]any{}, want: true},
		{dir: "/tasks/draft-x", metadata: map[string]any{}, want: false},
		{dir: "/tasks/local-legacy", metadata: map[string]any{}, want: true},
		{dir: "/tasks/local-", metadata: map[string]any{}, want: false},
	}
	for _, test := range tests {
		if got := IsLocalDraft(test.dir, test.metadata); got != test.want {
			t.Errorf("IsLocalDraft(%q, %#v) = %v, want %v", test.dir, test.metadata, got, test.want)
		}
	}
}

func TestNoteFilenameSanitizesAuthorAndSuffixesDuplicates(t *testing.T) {
	stamp := time.Date(2026, 11, 16, 10, 20, 30, 0, time.UTC)
	if got := NoteFilename(stamp, "Ada Lovelace!", 1); got != "20261116T102030.000Z-Ada-Lovelace.md" {
		t.Fatalf("filename = %q", got)
	}
	if got := NoteFilename(stamp, "", 2); got != "20261116T102030.000Z-unknown-2.md" {
		t.Fatalf("duplicate filename = %q", got)
	}
}

func TestUniqueNoteFilenameSkipsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	stamp := time.Date(2026, 11, 16, 10, 20, 30, 0, time.UTC)
	first, err := UniqueNoteFilename(dir, stamp, "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, first), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := UniqueNoteFilename(dir, stamp, "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("duplicate filename %q was reused", second)
	}
}
