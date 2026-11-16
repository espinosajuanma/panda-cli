package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func Pull(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	progress := NewPullProgress(out)
	defer progress.Finish()
	progress.Update(0)
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project labels: %w", err)
	}
	var reports []string
	labelConflicts, labelCount, err := pullProjectCatalogKind(ws, catdomain.Labels, labels, out, progress, 0, 25, &reports)
	if err != nil {
		return err
	}
	packages, err := client.ListPackages(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project packages: %w", err)
	}
	packageConflicts, packageCount, err := pullProjectCatalogKind(ws, catdomain.Packages, packages, out, progress, 25, 50, &reports)
	if err != nil {
		return err
	}
	members, err := client.ListProjectMembers(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project members: %w", err)
	}
	members = DedupeProjectMembers(members)
	if err := WriteProjectMembers(ws, members); err != nil {
		return fmt.Errorf("save project members: %w", err)
	}
	if len(members) == 0 {
		UpdatePullStage(progress, 50, 75, 0, 0)
	} else {
		for index := range members {
			UpdatePullStage(progress, 50, 75, index+1, len(members))
		}
	}
	tasks, err := client.ListTasks(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project tasks: %w", err)
	}
	if len(tasks) == 0 {
		progress.Update(95)
	}
	updated, created, conflictCount := 0, 0, labelConflicts+packageConflicts
	remoteIDs := make(map[string]bool, len(tasks))
	localByRemoteID := make(map[string]struct {
		dir      string
		metadata map[string]any
	})
	localDirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	for _, localDir := range localDirs {
		metadata, readErr := workspace.ReadMap(filepath.Join(localDir, "metadata.json"))
		if readErr == nil {
			if id := jsonutil.StringField(metadata, "id"); id != "" {
				localByRemoteID[id] = struct {
					dir      string
					metadata map[string]any
				}{localDir, metadata}
			}
		}
	}
	for taskIndex, remote := range tasks {
		id := jsonutil.StringField(remote, "id")
		if id == "" {
			UpdatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		remoteIDs[id] = true
		localEntry, exists := localByRemoteID[id]
		dir, local := localEntry.dir, localEntry.metadata
		if !exists {
			dir = ws.TaskPath(workspace.DirectoryName(remote))
			if _, statErr := os.Stat(dir); statErr == nil {
				dir = ws.TaskPath(workspace.DirectoryName(remote) + "-" + id)
			}
			if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
				return err
			}
			if err := WriteRemoteToLocal(dir, remote); err != nil {
				return fmt.Errorf("write task %s: %w", id, err)
			}
			state := workspace.State{Remote: jsonutil.CloneMap(remote)}
			if err := SyncRemoteNotes(dir, remote, state.StagedNotes, nil, nil); err != nil {
				return err
			}
			if err := ws.WriteState(id, state); err != nil {
				return err
			}
			created++
			UpdatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		state, err := ws.ReadState(id)
		if err != nil {
			return fmt.Errorf("task %s has no sync baseline; run clean then pull: %w", id, err)
		}
		if SameTaskVersion(remote, state.Remote) {
			if err := EnsureReleaseNotesFiles(dir, remote); err != nil {
				return err
			}
			UpdatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		fields := PullMetadataConflicts(local, state.Remote, remote)
		if len(fields) != 0 {
			conflictCount += len(fields)
			reports = append(reports, fmt.Sprintf("%s task %s: %s", ui.ColorText(out, "1;31", "Conflict"), task.Label(local), strings.Join(fields, ", ")))
		}
		merged := workspace.MetadataFromRemote(remote)
		for _, field := range task.EditableFields {
			if slices.Contains(fields, field) || (!jsonutil.Same(local[field], state.Remote[field]) && jsonutil.Same(remote[field], state.Remote[field])) {
				if value, exists := local[field]; exists {
					merged[field] = value
				} else {
					delete(merged, field)
				}
			}
		}
		if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), merged); err != nil {
			return err
		}
		baseHTML := jsonutil.StringField(state.Remote, "description")
		remoteHTML := jsonutil.StringField(remote, "description")
		baseMarkdown, err := content.HTMLToMarkdown(baseHTML)
		if err != nil {
			return err
		}
		remoteMarkdown, err := content.HTMLToMarkdown(remoteHTML)
		if err != nil {
			return err
		}
		localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
		if err != nil {
			return err
		}
		descriptionConflict := string(localMarkdown) != baseMarkdown && remoteHTML != baseHTML && string(localMarkdown) != remoteMarkdown
		if descriptionConflict {
			conflictCount++
			reports = append(reports, fmt.Sprintf("%s task %s: description", ui.ColorText(out, "1;31", "Conflict"), task.Label(local)))
		} else if string(localMarkdown) == baseMarkdown {
			if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(remoteMarkdown), 0o600); err != nil {
				return err
			}
		}
		localNoteNames, err := ChangedNoteFilenames(dir, state.Remote)
		if err != nil {
			return err
		}
		noteConflicts, err := LocalNoteConflicts(dir, state.Remote, remote)
		if err != nil {
			return err
		}
		if len(noteConflicts) > 0 {
			conflictCount += len(noteConflicts)
			labels := make([]string, len(noteConflicts))
			for index, filename := range noteConflicts {
				labels[index] = filepath.ToSlash(filepath.Join("notes", filename))
			}
			reports = append(reports, fmt.Sprintf("%s task %s: %s", ui.ColorText(out, "1;31", "Conflict"), task.Label(local), strings.Join(labels, ", ")))
		}
		if err := SyncRemoteNotes(dir, remote, state.StagedNotes, localNoteNames, state.Remote); err != nil {
			return err
		}
		releaseConflicts, err := ReleaseNoteConflicts(dir, state.Remote, remote)
		if err != nil {
			return err
		}
		if len(releaseConflicts) > 0 {
			conflictCount += len(releaseConflicts)
			reports = append(reports, fmt.Sprintf("%s task %s: %s", ui.ColorText(out, "1;31", "Conflict"), task.Label(local), strings.Join(releaseConflicts, ", ")))
		}
		if err := SyncReleaseNotesFromPull(dir, state.Remote, remote, releaseConflicts); err != nil {
			return err
		}
		baseline := state.Remote
		state.Remote = AdvanceBaseline(baseline, remote, fields, descriptionConflict)
		state.Remote = AdvanceNotesBaseline(baseline, state.Remote, noteConflicts)
		state.Remote = AdvanceReleaseNotesBaseline(baseline, state.Remote, releaseConflicts)
		if err := ws.WriteState(id, state); err != nil {
			return err
		}
		updated++
		UpdatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
	}
	missingRemote := 0
	for _, dir := range localDirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		id := jsonutil.StringField(metadata, "id")
		if id == "" || remoteIDs[id] {
			continue
		}
		reports = append(reports, fmt.Sprintf("%s task %s is no longer returned for this project; kept local files.", ui.ColorText(out, "1;33", "Warning:"), task.Label(metadata)))
		missingRemote++
	}
	progress.Update(100)
	progress.Finish()
	for _, report := range reports {
		fmt.Fprintln(out, report)
	}
	fmt.Fprintf(out, "%s project data: %d task(s): %d new, %d updated; %d labels, %d packages, %d members synchronized", ui.ColorText(out, "1;32", "Pulled"), len(tasks), created, updated, labelCount, packageCount, len(members))
	if missingRemote > 0 {
		fmt.Fprintf(out, "; %d missing remotely", missingRemote)
	}
	if conflictCount > 0 {
		fmt.Fprintf(out, "; %d conflict(s) need attention", conflictCount)
	}
	fmt.Fprintln(out)
	if conflictCount > 0 {
		return fmt.Errorf("pull completed with %d conflict(s); use `panda clean <taskNumber>` then pull again to discard local edits, or resolve the conflicting files manually", conflictCount)
	}
	return nil
}

func WriteProjectMembers(ws *workspace.Workspace, members []map[string]any) error {
	return ws.WriteMembers(DedupeProjectMembers(members))
}

func DedupeProjectMembers(members []map[string]any) []map[string]any {
	type membership struct{ userID, role string }
	seen := make(map[membership]bool, len(members))
	unique := make([]map[string]any, 0, len(members))
	for _, member := range members {
		user, _ := member["user"].(map[string]any)
		id := jsonutil.RelationshipID(user)
		role := strings.TrimSpace(jsonutil.StringField(member, "role"))
		key := membership{userID: id, role: role}
		if id != "" && role != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		unique = append(unique, member)
	}
	return unique
}

func SameTaskVersion(remote, baseline map[string]any) bool {
	return task.SameVersion(task.FromMap(remote), task.FromMap(baseline))
}

type PullProgress struct {
	out     io.Writer
	current int
	active  bool
	drawn   bool
}

func NewPullProgress(out io.Writer) *PullProgress {
	return &PullProgress{out: out, active: ui.IsTerminal(out)}
}

func (p *PullProgress) Update(percent int) {
	if !p.active {
		return
	}
	p.current = min(max(percent, 0), 100)
	fmt.Fprintf(p.out, "\r%s", ui.ColorText(p.out, "1;36", FormatPullProgress(p.current)))
	p.drawn = true
}

func UpdatePullStage(progress *PullProgress, start, end, current, total int) {
	if total <= 0 {
		progress.Update(end)
		return
	}
	percent := start + (end-start)*min(max(current, 0), total)/total
	progress.Update(percent)
}

func (p *PullProgress) Finish() {
	if p.active && p.drawn {
		fmt.Fprintln(p.out)
		p.drawn = false
	}
}

func FormatPullProgress(percent int) string {
	percent = min(max(percent, 0), 100)
	const width = 20
	filled := percent * width / 100
	return fmt.Sprintf("Pulling project data: [%s%s] %d%%", strings.Repeat("#", filled), strings.Repeat("-", width-filled), percent)
}
