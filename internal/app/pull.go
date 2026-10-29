package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func pull(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	progress := newPullProgress(out)
	defer progress.Finish()
	progress.Update(0)
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project labels: %w", err)
	}
	var reports []string
	labelConflicts, labelCount, err := pullProjectCatalogKind(ws, labelCatalog, labels, out, progress, 0, 25, &reports)
	if err != nil {
		return err
	}
	packages, err := client.ListPackages(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project packages: %w", err)
	}
	packageConflicts, packageCount, err := pullProjectCatalogKind(ws, packageCatalog, packages, out, progress, 25, 50, &reports)
	if err != nil {
		return err
	}
	members, err := client.ListProjectMembers(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("fetch project members: %w", err)
	}
	members = dedupeProjectMembers(members)
	if err := writeProjectMembers(ws, members); err != nil {
		return fmt.Errorf("save project members: %w", err)
	}
	if len(members) == 0 {
		updatePullStage(progress, 50, 75, 0, 0)
	} else {
		for index := range members {
			updatePullStage(progress, 50, 75, index+1, len(members))
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
	for taskIndex, remote := range tasks {
		id := stringField(remote, "id")
		if id == "" {
			updatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		remoteIDs[id] = true
		dir, local, err := ws.FindByRemoteID(id)
		if errors.Is(err, os.ErrNotExist) {
			dir = ws.TaskPath(workspace.DirectoryName(remote))
			if _, statErr := os.Stat(dir); statErr == nil {
				dir = ws.TaskPath(workspace.DirectoryName(remote) + "-" + id)
			}
			if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
				return err
			}
			local = workspace.MetadataFromRemote(remote)
			if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
				return err
			}
			markdown, err := content.HTMLToMarkdown(stringField(remote, "description"))
			if err != nil {
				return fmt.Errorf("convert task %s description: %w", id, err)
			}
			if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600); err != nil {
				return err
			}
			state := workspace.State{Remote: cloneMap(remote)}
			if err := syncRemoteNotes(dir, remote, state.StagedNotes, nil); err != nil {
				return err
			}
			if err := ws.WriteState(id, state); err != nil {
				return err
			}
			created++
			updatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		if err != nil {
			return err
		}
		state, err := ws.ReadState(id)
		if err != nil {
			return fmt.Errorf("task %s has no sync baseline; run clean then pull: %w", id, err)
		}
		if sameTaskVersion(remote, state.Remote) {
			updatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
			continue
		}
		fields := pullMetadataConflicts(local, state.Remote, remote)
		if len(fields) != 0 {
			conflictCount += len(fields)
			reports = append(reports, fmt.Sprintf("%s task %s: %s", colorText(out, "1;31", "Conflict"), taskLabel(local), strings.Join(fields, ", ")))
		}
		merged := workspace.MetadataFromRemote(remote)
		for _, field := range editableFields {
			if isConflictField(fields, field) || (!same(valueAt(local, field), valueAt(state.Remote, field)) && same(valueAt(remote, field), valueAt(state.Remote, field))) {
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
		baseHTML := stringField(state.Remote, "description")
		remoteHTML := stringField(remote, "description")
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
			reports = append(reports, fmt.Sprintf("%s task %s: description", colorText(out, "1;31", "Conflict"), taskLabel(local)))
		} else if string(localMarkdown) == baseMarkdown {
			if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(remoteMarkdown), 0o600); err != nil {
				return err
			}
		}
		localNoteNames, err := changedNoteFilenames(dir, state.Remote)
		if err != nil {
			return err
		}
		noteConflicts, err := localNoteConflicts(dir, state.Remote, remote)
		if err != nil {
			return err
		}
		if len(noteConflicts) > 0 {
			conflictCount += len(noteConflicts)
			labels := make([]string, len(noteConflicts))
			for index, filename := range noteConflicts {
				labels[index] = filepath.ToSlash(filepath.Join("notes", filename))
			}
			reports = append(reports, fmt.Sprintf("%s task %s: %s", colorText(out, "1;31", "Conflict"), taskLabel(local), strings.Join(labels, ", ")))
		}
		if err := syncRemoteNotes(dir, remote, state.StagedNotes, localNoteNames); err != nil {
			return err
		}
		baseline := state.Remote
		state.Remote = advanceBaseline(baseline, remote, fields, descriptionConflict)
		state.Remote = advanceNotesBaseline(baseline, state.Remote, noteConflicts)
		if err := ws.WriteState(id, state); err != nil {
			return err
		}
		updated++
		updatePullStage(progress, 75, 95, taskIndex+1, len(tasks))
	}
	missingRemote := 0
	localDirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	for _, dir := range localDirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		id := stringField(metadata, "id")
		if id == "" || remoteIDs[id] {
			continue
		}
		reports = append(reports, fmt.Sprintf("%s task %s is no longer returned for this project; kept local files.", colorText(out, "1;33", "Warning:"), taskLabel(metadata)))
		missingRemote++
	}
	progress.Update(100)
	progress.Finish()
	for _, report := range reports {
		fmt.Fprintln(out, report)
	}
	fmt.Fprintf(out, "%s project data: %d task(s): %d new, %d updated; %d labels, %d packages, %d members synchronized", colorText(out, "1;32", "Pulled"), len(tasks), created, updated, labelCount, packageCount, len(members))
	if missingRemote > 0 {
		fmt.Fprintf(out, "; %d missing remotely", missingRemote)
	}
	if conflictCount > 0 {
		fmt.Fprintf(out, "; %d conflict(s) need attention", conflictCount)
	}
	fprintln(out)
	if conflictCount > 0 {
		return fmt.Errorf("pull completed with %d conflict(s); use `panda clean <taskNumber>` then pull again to discard local edits, or resolve the conflicting files manually", conflictCount)
	}
	return nil
}

func writeProjectMembers(ws *workspace.Workspace, members []map[string]any) error {
	members = dedupeProjectMembers(members)
	data, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		return err
	}
	return workspace.WriteFileAtomic(filepath.Join(ws.Root, ".panda", "members.json"), append(data, '\n'), 0o600)
}

func dedupeProjectMembers(members []map[string]any) []map[string]any {
	type membership struct{ userID, role string }
	seen := make(map[membership]bool, len(members))
	unique := make([]map[string]any, 0, len(members))
	for _, member := range members {
		user, _ := member["user"].(map[string]any)
		id := relationshipID(user)
		role := strings.TrimSpace(stringField(member, "role"))
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

func sameTaskVersion(remote, baseline map[string]any) bool {
	remoteVersion, remoteHasVersion := remote["version"]
	baseVersion, baselineHasVersion := baseline["version"]
	return remoteHasVersion && baselineHasVersion && remoteVersion != nil && baseVersion != nil &&
		same(remoteVersion, baseVersion) && same(valueAt(remote, "labels"), valueAt(baseline, "labels")) &&
		same(valueAt(remote, "package"), valueAt(baseline, "package"))
}

type pullProgress struct {
	out     io.Writer
	current int
	active  bool
	drawn   bool
}

func newPullProgress(out io.Writer) *pullProgress {
	return &pullProgress{out: out, active: isTerminal(out)}
}

func (p *pullProgress) Update(percent int) {
	if !p.active {
		return
	}
	p.current = min(max(percent, 0), 100)
	fmt.Fprintf(p.out, "\r%s", colorText(p.out, "1;36", formatPullProgress(p.current)))
	p.drawn = true
}

func updatePullStage(progress *pullProgress, start, end, current, total int) {
	if total <= 0 {
		progress.Update(end)
		return
	}
	percent := start + (end-start)*min(max(current, 0), total)/total
	progress.Update(percent)
}

func (p *pullProgress) Finish() {
	if p.active && p.drawn {
		fmt.Fprintln(p.out)
		p.drawn = false
	}
}

func formatPullProgress(percent int) string {
	percent = min(max(percent, 0), 100)
	const width = 20
	filled := percent * width / 100
	return fmt.Sprintf("Pulling project data: [%s%s] %d%%", strings.Repeat("#", filled), strings.Repeat("-", width-filled), percent)
}
