package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	catdomain "github.com/espinosajuanma/panda-cli/internal/catalog"
	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func Push(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	return PushSelected(ctx, client, ws, project, nil, out)
}

func PushSelected(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, selectors []string, out io.Writer) error {
	return PushSelectedWithOptions(ctx, client, ws, project, PushOptions{Selectors: selectors}, out)
}

func PushSelectedWithOptions(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, options PushOptions, out io.Writer) error {
	dirs, err := pushTaskDirectories(ws, options.Selectors)
	if err != nil {
		return err
	}
	reportDoc := report.PushReport{
		SchemaVersion: 1,
		Tasks:         make([]report.PushTaskResult, 0, len(dirs)),
		Errors:        make([]string, 0),
	}
	fatal := func(fatalErr error) error {
		if options.JSON {
			reportDoc.Errors = append(reportDoc.Errors, fatalErr.Error())
			reportDoc.UpdateSummary()
			if renderErr := report.RenderPushJSON(reportDoc, out); renderErr != nil {
				return renderErr
			}
		}
		return fatalErr
	}
	labelIDs, labelPlan, labelChanged, err := PushProjectCatalog(ctx, client, ws, project, catdomain.Labels)
	if err != nil {
		return fatal(err)
	}
	reportDoc.Catalogs.Labels = catalogPushResult(labelPlan, labelChanged)
	packageIDs, packagePlan, packageChanged, err := PushProjectCatalog(ctx, client, ws, project, catdomain.Packages)
	if err != nil {
		return fatal(err)
	}
	reportDoc.Catalogs.Packages = catalogPushResult(packagePlan, packageChanged)
	catalogIDs := make(map[string]string, len(labelIDs)+len(packageIDs))
	for localID, remoteID := range labelIDs {
		catalogIDs[localID] = remoteID
	}
	for localID, remoteID := range packageIDs {
		catalogIDs[localID] = remoteID
	}
	if err := RewriteTaskCatalogIDs(ws, catalogIDs); err != nil {
		return fatal(err)
	}
	catalogChanged := labelChanged || packageChanged
	catalogPlans := []*CatalogPushPlan{labelPlan, packagePlan}
	var problems []string
	pushed := 0
	for _, dir := range dirs {
		local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			result := report.NewPushTaskResult(nil, filepath.Base(dir))
			recordTaskProblem(&reportDoc, &problems, &result, "failed", err.Error())
			continue
		}
		result := report.NewPushTaskResult(local, filepath.Base(dir))
		recordFailure := func(outcome, message string) {
			recordTaskProblem(&reportDoc, &problems, &result, outcome, message)
		}
		state, key, err := ws.LoadTaskState(dir, local)
		if err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		if !state.LocalCreated && jsonutil.StringField(local, "id") == "" && jsonutil.StringField(state.Remote, "id") != "" {
			dir, local, key, err = RecoverCreatedTask(ws, dir, local, key, state)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("recover created task: %v", err))
				continue
			}
			updatePushTaskIdentity(&result, local)
		}
		dirty, err := TaskHasLocalChanges(local, state, dir)
		if err != nil {
			recordFailure("failed", fmt.Sprintf("inspect local changes: %v", err))
			continue
		}
		if !dirty {
			result.Outcome = "skipped"
			reportDoc.Tasks = append(reportDoc.Tasks, result)
			continue
		}
		if err := ValidateStagedFiles(state.StagedFiles); err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		createdTask := false
		if state.LocalCreated {
			result.AddAction("create")
			remote, err := createRemoteTask(ctx, client, dir, local, project, catalogIDs)
			if err != nil {
				recordFailure("failed", err.Error())
				continue
			}
			result.RemoteCreated = true
			updatePushTaskIdentity(&result, remote)
			newID := jsonutil.StringField(remote, "id")
			if newID == "" {
				recordFailure("failed", "task was created remotely but the API response omitted its ID")
				continue
			}
			createdTask = true
			state.Remote = jsonutil.CloneMap(remote)
			state.LocalCreated = false
			state.StagedDiscard = false
			dir, local, key, err = RecoverCreatedTask(ws, dir, local, key, state)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("save created task locally: %v", err))
				continue
			}
			updatePushTaskIdentity(&result, local)
			if !options.JSON {
				fmt.Fprintf(out, "Created task %s.\n", task.Label(local))
			}
		} else {
			id := jsonutil.StringField(local, "id")
			remote, err := client.GetTask(ctx, id)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("fetch current remote task: %v", err))
				continue
			}
			if projectID := jsonutil.RelationshipID(remote["project"]); projectID != "" && projectID != project.ID {
				recordFailure("failed", fmt.Sprintf("task now belongs to project %s", projectID))
				continue
			}
			conflicts := TaskConflicts(local, state.Remote, remote, dir)
			if len(conflicts) > 0 {
				recordFailure("conflict", fmt.Sprintf("conflict in %s; use `panda clean %s` then pull to take the remote version, or resolve the files manually", strings.Join(conflicts, ", "), task.Label(local)))
				continue
			}
			noteChanges, err := ChangedNoteFilenames(dir, state.Remote)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("inspect edited notes: %v", err))
				continue
			}
			var myNotes []map[string]any
			if len(noteChanges) > 0 {
				myNotes, err = BuildMyNotesPayload(ctx, client, dir, remote, state.Remote)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("edit notes: %v", err))
					continue
				}
			}
			localForUpdate := local
			if state.StagedDiscard {
				localForUpdate = jsonutil.CloneMap(local)
				localForUpdate["status"] = jsonutil.ValueAt(state.Remote, "status")
			}
			payload, changed, err := MergeForPush(localForUpdate, remote, state.Remote, dir)
			if err != nil {
				recordFailure("failed", err.Error())
				continue
			}
			payload, err = RemapTaskCatalogRelationships(payload, catalogIDs)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("map task labels/packages: %v", err))
				continue
			}
			if changed {
				result.AddAction("fields")
				remote, err = client.UpdateTask(ctx, id, payload)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("update: %v", err))
					continue
				}
			}
			if state.StagedDiscard {
				result.AddAction("discard")
				if jsonutil.StringField(remote, "status") != "discarded" {
					remote, err = client.DiscardTask(ctx, id)
					if err != nil {
						recordFailure("failed", fmt.Sprintf("discard: %v", err))
						continue
					}
					if jsonutil.StringField(remote, "id") != id || jsonutil.StringField(remote, "status") != "discarded" {
						remote, err = client.GetTask(ctx, id)
						if err != nil {
							recordFailure("failed", fmt.Sprintf("verify discard: %v", err))
							continue
						}
					}
					if jsonutil.StringField(remote, "status") != "discarded" {
						recordFailure("failed", fmt.Sprintf("discard action completed but status is %q", jsonutil.StringField(remote, "status")))
						continue
					}
				}
				state.StagedDiscard = false
			}
			if len(myNotes) > 0 {
				result.AddAction("editedNotes")
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
				if err := SyncRemoteNotes(dir, remote, state.StagedNotes, nil, state.Remote); err != nil {
					recordFailure("failed", fmt.Sprintf("refresh edited notes: %v", err))
					continue
				}
			}
			releasePayload, err := ReleaseNotesPayload(dir, state.Remote)
			if err != nil {
				recordFailure("failed", fmt.Sprintf("edit release notes: %v", err))
				continue
			}
			if len(releasePayload) > 0 {
				result.AddAction("releaseNotes")
				remote, err = client.EditReleaseNotes(ctx, id, releasePayload)
				if err != nil {
					recordFailure("failed", fmt.Sprintf("edit release notes: %v", err))
					continue
				}
				if remote == nil || remote["releaseNotes"] == nil {
					remote, err = client.GetTask(ctx, id)
					if err != nil {
						recordFailure("failed", fmt.Sprintf("refresh edited release notes: %v", err))
						continue
					}
				}
			}
			state.Remote = jsonutil.CloneMap(remote)
			if err := WriteRemoteToLocal(dir, remote); err != nil {
				recordFailure("failed", fmt.Sprintf("update local baseline: %v", err))
				continue
			}
		}
		if len(state.StagedNotes) > 0 {
			result.AddAction("stagedNotes")
		}
		state, err = PushStagedNotes(ctx, client, ws, dir, local, key, state)
		if err != nil {
			recordFailure("failed", err.Error())
			continue
		}
		if len(state.StagedFiles) > 0 {
			result.AddAction("files")
		}
		state, err = PushStagedFiles(ctx, client, ws, dir, local, key, state)
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
		reportDoc.Tasks = append(reportDoc.Tasks, result)
		if !options.JSON {
			fmt.Fprintf(out, "%s %s.\n", ui.ColorText(out, "1;32", "Pushed"), task.Label(local))
		}
		pushed++
	}
	if len(problems) == 0 {
		if err := PushProjectCatalogDeletes(ctx, client, ws, project, catalogPlans); err != nil {
			problems = append(problems, err.Error())
			reportDoc.Errors = append(reportDoc.Errors, err.Error())
		}
	}
	reportDoc.Catalogs.Labels = catalogPushResult(labelPlan, labelChanged)
	reportDoc.Catalogs.Packages = catalogPushResult(packagePlan, packageChanged)
	if options.JSON {
		reportDoc.UpdateSummary()
		if err := report.RenderPushJSON(reportDoc, out); err != nil {
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
		if len(options.Selectors) > 0 {
			fmt.Fprintln(out, "No local changes to push for the selected task(s).")
		} else {
			fmt.Fprintln(out, "No local changes to push.")
		}
	}
	return nil
}

func RecoverCreatedTask(ws *workspace.Workspace, dir string, previous map[string]any, oldKey string, state workspace.State) (string, map[string]any, string, error) {
	remoteID := jsonutil.StringField(state.Remote, "id")
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

func TaskHasLocalChanges(local map[string]any, state workspace.State, dir string) (bool, error) {
	if state.LocalCreated || state.StagedDiscard || len(state.StagedNotes) > 0 || len(state.StagedFiles) > 0 {
		return true, nil
	}
	for _, field := range task.EditableFields {
		if !jsonutil.Same(jsonutil.ValueAt(local, field), jsonutil.ValueAt(state.Remote, field)) {
			return true, nil
		}
	}
	baselineMarkdown, err := content.HTMLToMarkdown(jsonutil.StringField(state.Remote, "description"))
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
	noteChanges, err := LocalNoteChanges(dir, state.Remote)
	if err != nil {
		return false, err
	}
	if len(noteChanges) > 0 {
		return true, nil
	}
	releaseChanges, err := ReleaseNoteChanges(dir, state.Remote)
	if err != nil {
		return false, err
	}
	return len(releaseChanges) > 0, nil
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
	newTask := jsonutil.CloneMap(metadata)
	newTask, err = RemapTaskCatalogRelationships(newTask, catalogIDs)
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
	delete(newTask, "releaseNotes")
	localReleaseNotes, err := ReadLocalReleaseNotes(dir)
	if err != nil {
		return nil, err
	}
	releaseNotes := make(map[string]any)
	for _, field := range ReleaseNoteFields {
		if !localReleaseNotes.Exists[field.Name] || strings.TrimSpace(localReleaseNotes.Markdown[field.Name]) == "" {
			continue
		}
		fieldHTML, err := content.MarkdownToHTML(localReleaseNotes.Markdown[field.Name])
		if err != nil {
			return nil, fmt.Errorf("convert %s: %w", field.Name, err)
		}
		releaseNotes[field.Name] = fieldHTML
	}
	if len(releaseNotes) > 0 {
		newTask["releaseNotes"] = releaseNotes
	}
	return client.CreateTask(ctx, newTask)
}
