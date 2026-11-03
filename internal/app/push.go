package app

import (
	"context"
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

func push(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	return pushSelected(ctx, client, ws, project, nil, out)
}

func pushSelected(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, selectors []string, out io.Writer) error {
	return pushSelectedWithOptions(ctx, client, ws, project, pushOptions{selectors: selectors}, out)
}

func pushSelectedWithOptions(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, options pushOptions, out io.Writer) error {
	dirs, err := pushTaskDirectories(ws, options.selectors)
	if err != nil {
		return err
	}
	report := pushReport{
		SchemaVersion: 1,
		Tasks:         make([]pushTaskResult, 0, len(dirs)),
		Errors:        make([]string, 0),
	}
	fatal := func(fatalErr error) error {
		if options.json {
			report.Errors = append(report.Errors, fatalErr.Error())
			report.updateSummary()
			if renderErr := renderPushJSON(report, out); renderErr != nil {
				return renderErr
			}
		}
		return fatalErr
	}
	labelIDs, labelPlan, labelChanged, err := pushProjectCatalog(ctx, client, ws, project, labelCatalog)
	if err != nil {
		return fatal(err)
	}
	report.Catalogs.Labels = catalogPushResult(labelPlan, labelChanged)
	packageIDs, packagePlan, packageChanged, err := pushProjectCatalog(ctx, client, ws, project, packageCatalog)
	if err != nil {
		return fatal(err)
	}
	report.Catalogs.Packages = catalogPushResult(packagePlan, packageChanged)
	catalogIDs := make(map[string]string, len(labelIDs)+len(packageIDs))
	for localID, remoteID := range labelIDs {
		catalogIDs[localID] = remoteID
	}
	for localID, remoteID := range packageIDs {
		catalogIDs[localID] = remoteID
	}
	if err := rewriteTaskCatalogIDs(ws, catalogIDs); err != nil {
		return fatal(err)
	}
	catalogChanged := labelChanged || packageChanged
	catalogPlans := []*catalogPushPlan{labelPlan, packagePlan}
	var problems []string
	pushed := 0
	for _, dir := range dirs {
		local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			result := newPushTaskResult(nil, filepath.Base(dir))
			recordTaskProblem(&report, &problems, &result, "failed", err.Error())
			continue
		}
		result := newPushTaskResult(local, filepath.Base(dir))
		recordFailure := func(outcome, message string) {
			recordTaskProblem(&report, &problems, &result, outcome, message)
		}
		state, key, err := loadTaskState(ws, dir, local)
		if err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		if !state.LocalCreated && stringField(local, "id") == "" && stringField(state.Remote, "id") != "" {
			dir, local, key, err = recoverCreatedTask(ws, dir, local, key, state)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("recover created task: %v", err))
				continue
			}
			updatePushTaskIdentity(&result, local)
		}
		dirty, err := taskHasLocalChanges(local, state, dir)
		if err != nil {
			recordFailure("failed", fmt.Sprintf("inspect local changes: %v", err))
			continue
		}
		if !dirty {
			result.Outcome = "skipped"
			report.Tasks = append(report.Tasks, result)
			continue
		}
		if err := validateStagedFiles(state.StagedFiles); err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		createdTask := false
		if state.LocalCreated {
			result.addAction("create")
			remote, err := createRemoteTask(ctx, client, dir, local, project, catalogIDs)
			if err != nil {
				recordFailure("failed", err.Error())
				continue
			}
			result.RemoteCreated = true
			updatePushTaskIdentity(&result, remote)
			newID := stringField(remote, "id")
			if newID == "" {
				recordFailure("failed", "task was created remotely but the API response omitted its ID")
				continue
			}
			createdTask = true
			state.Remote = cloneMap(remote)
			state.LocalCreated = false
			state.StagedDiscard = false
			dir, local, key, err = recoverCreatedTask(ws, dir, local, key, state)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("save created task locally: %v", err))
				continue
			}
			updatePushTaskIdentity(&result, local)
			if !options.json {
				fmt.Fprintf(out, "Created task %s.\n", taskLabel(local))
			}
		} else {
			id := stringField(local, "id")
			remote, err := client.GetTask(ctx, id)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("fetch current remote task: %v", err))
				continue
			}
			if projectID := relationshipID(remote["project"]); projectID != "" && projectID != project.ID {
				recordFailure("failed", fmt.Sprintf("task now belongs to project %s", projectID))
				continue
			}
			conflicts := taskConflicts(local, state.Remote, remote, dir)
			if len(conflicts) > 0 {
				recordFailure("conflict", fmt.Sprintf("conflict in %s; use `panda clean %s` then pull to take the remote version, or resolve the files manually", strings.Join(conflicts, ", "), taskLabel(local)))
				continue
			}
			noteChanges, err := changedNoteFilenames(dir, state.Remote)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("inspect edited notes: %v", err))
				continue
			}
			var myNotes []map[string]any
			if len(noteChanges) > 0 {
				myNotes, err = buildMyNotesPayload(ctx, client, dir, remote, state.Remote)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("edit notes: %v", err))
					continue
				}
			}
			localForUpdate := local
			if state.StagedDiscard {
				localForUpdate = cloneMap(local)
				localForUpdate["status"] = valueAt(state.Remote, "status")
			}
			payload, changed, err := mergeForPush(localForUpdate, remote, state.Remote, dir)
			if err != nil {
				recordFailure("failed", err.Error())
				continue
			}
			payload, err = remapTaskCatalogRelationships(payload, catalogIDs)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("map task labels/packages: %v", err))
				continue
			}
			if changed {
				result.addAction("fields")
				remote, err = client.UpdateTask(ctx, id, payload)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("update: %v", err))
					continue
				}
			}
			if state.StagedDiscard {
				result.addAction("discard")
				if stringField(remote, "status") != "discarded" {
					remote, err = client.DiscardTask(ctx, id)
					if err != nil {
						recordFailure("failed", fmt.Sprintf("discard: %v", err))
						continue
					}
					if stringField(remote, "id") != id || stringField(remote, "status") != "discarded" {
						remote, err = client.GetTask(ctx, id)
						if err != nil {
							recordFailure("failed", fmt.Sprintf("verify discard: %v", err))
							continue
						}
					}
					if stringField(remote, "status") != "discarded" {
						recordFailure("failed", fmt.Sprintf("discard action completed but status is %q", stringField(remote, "status")))
						continue
					}
				}
				state.StagedDiscard = false
			}
			if len(myNotes) > 0 {
				result.addAction("editedNotes")
				remote, err = client.EditMyNotes(ctx, id, myNotes)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("edit notes: %v", err))
					continue
				}
				if remote == nil || remote["notes"] == nil {
					remote, err = client.GetTask(ctx, id)
					if err != nil {
						recordFailure("failed", fmt.Sprintf("refresh edited notes: %v", err))
						continue
					}
				}
				if err := syncRemoteNotes(dir, remote, state.StagedNotes, nil, state.Remote); err != nil {
					recordFailure("failed", fmt.Sprintf("refresh edited notes: %v", err))
					continue
				}
			}
			state.Remote = cloneMap(remote)
			if err := writeRemoteToLocal(dir, remote); err != nil {
				recordFailure("failed", fmt.Sprintf("update local baseline: %v", err))
				continue
			}
		}
		if len(state.StagedNotes) > 0 {
			result.addAction("stagedNotes")
		}
		state, err = pushStagedNotes(ctx, client, ws, dir, local, key, state)
		if err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		if len(state.StagedFiles) > 0 {
			result.addAction("files")
		}
		state, err = pushStagedFiles(ctx, client, ws, dir, local, key, state)
		if err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		if createdTask {
			result.Outcome = "created"
		} else {
			result.Outcome = "pushed"
		}
		updatePushTaskIdentity(&result, local)
		report.Tasks = append(report.Tasks, result)
		if !options.json {
			fmt.Fprintf(out, "%s %s.\n", colorText(out, "1;32", "Pushed"), taskLabel(local))
		}
		pushed++
	}
	if len(problems) == 0 {
		if err := pushProjectCatalogDeletes(ctx, client, ws, project, catalogPlans); err != nil {
			problems = append(problems, err.Error())
			report.Errors = append(report.Errors, err.Error())
		}
	}
	report.Catalogs.Labels = catalogPushResult(labelPlan, labelChanged)
	report.Catalogs.Packages = catalogPushResult(packagePlan, packageChanged)
	if options.json {
		report.updateSummary()
		if err := renderPushJSON(report, out); err != nil {
			return err
		}
		if len(problems) > 0 {
			return fmt.Errorf("push had %d failure(s)", len(problems))
		}
		return nil
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(out, "Error: %s\n", problem)
		}
		return fmt.Errorf("push had %d failure(s)", len(problems))
	}
	if catalogChanged {
		fmt.Fprintln(out, "Pushed project labels/packages.")
	}
	if pushed == 0 && !catalogChanged {
		if len(options.selectors) > 0 {
			fmt.Fprintln(out, "No local changes to push for the selected task(s).")
		} else {
			fmt.Fprintln(out, "No local changes to push.")
		}
	}
	return nil
}

func recoverCreatedTask(ws *workspace.Workspace, dir string, previous map[string]any, oldKey string, state workspace.State) (string, map[string]any, string, error) {
	remoteID := stringField(state.Remote, "id")
	if remoteID == "" {
		return dir, previous, oldKey, fmt.Errorf("remote baseline has no task ID")
	}
	state.LocalCreated = false
	if err := ws.WriteState(oldKey, state); err != nil {
		return dir, previous, oldKey, err
	}
	if err := ws.WriteState(remoteID, state); err != nil {
		return dir, previous, oldKey, err
	}
	metadata := workspace.MetadataFromRemote(state.Remote)
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return dir, previous, oldKey, err
	}
	if oldKey != remoteID {
		_ = os.Remove(ws.StatePathForKey(oldKey))
	}
	newDir := ws.TaskPath(workspace.DirectoryName(state.Remote))
	if _, err := os.Stat(newDir); err == nil && newDir != dir {
		newDir = ws.TaskPath(workspace.DirectoryName(state.Remote) + "-" + safeName(remoteID))
	}
	if newDir != dir {
		if err := os.Rename(dir, newDir); err == nil {
			dir = newDir
		}
	}
	return dir, metadata, remoteID, nil
}

func pushTaskDirectories(ws *workspace.Workspace, selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return ws.TaskDirectories()
	}
	dirs := make([]string, 0, len(selectors))
	seen := make(map[string]bool, len(selectors))
	for _, selector := range selectors {
		dir, _, err := ws.FindTask(selector)
		if err != nil {
			return nil, err
		}
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

func taskHasLocalChanges(local map[string]any, state workspace.State, dir string) (bool, error) {
	if state.LocalCreated || state.StagedDiscard || len(state.StagedNotes) > 0 || len(state.StagedFiles) > 0 {
		return true, nil
	}
	for _, field := range editableFields {
		if !same(valueAt(local, field), valueAt(state.Remote, field)) {
			return true, nil
		}
	}
	baselineMarkdown, err := content.HTMLToMarkdown(stringField(state.Remote, "description"))
	if err != nil {
		return false, err
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return false, err
	}
	if string(localMarkdown) != baselineMarkdown {
		return true, nil
	}
	noteChanges, err := localNoteChanges(dir, state.Remote)
	if err != nil {
		return false, err
	}
	return len(noteChanges) > 0, nil
}

func createRemoteTask(ctx context.Context, client *solutions.Client, dir string, metadata map[string]any, project *config.Project, catalogIDs map[string]string) (map[string]any, error) {
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return nil, err
	}
	html, err := content.MarkdownToHTML(string(description))
	if err != nil {
		return nil, err
	}
	newTask := cloneMap(metadata)
	newTask, err = remapTaskCatalogRelationships(newTask, catalogIDs)
	if err != nil {
		return nil, err
	}
	newTask["project"] = map[string]any{"id": project.ID, "label": project.Name}
	newTask["description"] = html
	delete(newTask, "id")
	delete(newTask, "localId")
	delete(newTask, "number")
	delete(newTask, "label")
	delete(newTask, "version")
	delete(newTask, "notes")
	return client.CreateTask(ctx, newTask)
}
