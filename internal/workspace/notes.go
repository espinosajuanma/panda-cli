package workspace

import (
	"errors"
	"os"
	"path/filepath"
)

type NoteFile struct {
	Filename string
	Markdown string
}

func TaskNoteFiles(dir string) ([]NoteFile, error) {
	notesDir := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	notes := make([]NoteFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(notesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		notes = append(notes, NoteFile{Filename: entry.Name(), Markdown: string(body)})
	}
	return notes, nil
}

func ReadNoteFile(dir, filename string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "notes", filename))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func WriteNoteFile(dir, filename, markdown string) error {
	return WriteFileAtomic(filepath.Join(dir, "notes", filename), []byte(markdown), 0o600)
}

func RemoveNoteFile(dir, filename string) error {
	err := os.Remove(filepath.Join(dir, "notes", filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
