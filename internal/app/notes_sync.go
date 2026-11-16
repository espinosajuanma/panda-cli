package app

import (
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
)

type remoteNote = syncpkg.RemoteNote

func writeRemoteToLocal(dir string, remote map[string]any) error {
	return syncpkg.WriteRemoteToLocal(dir, remote)
}

func getRemoteNotes(remote map[string]any) ([]remoteNote, error) {
	return syncpkg.GetRemoteNotes(remote)
}

func timestampMillis(value any) (int64, bool) {
	return syncpkg.TimestampMillis(value)
}

func noteOwnedBy(note remoteNote, user map[string]any, email string) bool {
	return syncpkg.NoteOwnedBy(note, user, email)
}

func localNoteChanges(dir string, baseline map[string]any) (map[string]string, error) {
	return syncpkg.LocalNoteChanges(dir, baseline)
}

func syncRemoteNotes(dir string, remote map[string]any, stagedNotes, preservedNotes []string, previous map[string]any) error {
	return syncpkg.SyncRemoteNotes(dir, remote, stagedNotes, preservedNotes, previous)
}

func baselineNoteMarkdown(remote map[string]any) (map[string]string, error) {
	return syncpkg.BaselineNoteMarkdown(remote)
}
