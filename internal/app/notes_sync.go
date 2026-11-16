package app

import (
	"context"

	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type remoteNote = syncpkg.RemoteNote

func advanceNotesBaseline(baseline, remote map[string]any, conflicts []string) map[string]any {
	return syncpkg.AdvanceNotesBaseline(baseline, remote, conflicts)
}

func writeRemoteToLocal(dir string, remote map[string]any) error {
	return syncpkg.WriteRemoteToLocal(dir, remote)
}

func buildMyNotesPayload(ctx context.Context, client *solutions.Client, dir string, remote, baseline map[string]any) ([]map[string]any, error) {
	return syncpkg.BuildMyNotesPayload(ctx, client, dir, remote, baseline)
}

func pushStagedNotes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, local map[string]any, key string, state workspace.State) (workspace.State, error) {
	return syncpkg.PushStagedNotes(ctx, client, ws, dir, local, key, state)
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

func localNoteConflicts(dir string, baseline, remote map[string]any) ([]string, error) {
	return syncpkg.LocalNoteConflicts(dir, baseline, remote)
}

func changedNoteFilenames(dir string, baseline map[string]any) ([]string, error) {
	return syncpkg.ChangedNoteFilenames(dir, baseline)
}

func syncRemoteNotes(dir string, remote map[string]any, stagedNotes, preservedNotes []string, previous map[string]any) error {
	return syncpkg.SyncRemoteNotes(dir, remote, stagedNotes, preservedNotes, previous)
}

func baselineNoteMarkdown(remote map[string]any) (map[string]string, error) {
	return syncpkg.BaselineNoteMarkdown(remote)
}
