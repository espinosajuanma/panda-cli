package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/content"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

func push(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	labelIDs, labelPlan, labelChanged, err := pushProjectCatalog(ctx, client, ws, project, labelCatalog)
	if err != nil {
		return err
	}
	packageIDs, packagePlan, packageChanged, err := pushProjectCatalog(ctx, client, ws, project, packageCatalog)
	if err != nil {
		return err
	}
	catalogIDs := make(map[string]string, len(labelIDs)+len(packageIDs))
	for localID, remoteID := range labelIDs {
		catalogIDs[localID] = remoteID
	}
	for localID, remoteID := range packageIDs {
		catalogIDs[localID] = remoteID
	}
	if err := rewriteTaskCatalogIDs(ws, catalogIDs); err != nil {
		return err
	}
	catalogChanged := labelChanged || packageChanged
	catalogPlans := []*catalogPushPlan{labelPlan, packagePlan}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	var problems []string
	pushed := 0
	for _, dir := range dirs {
		local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", filepath.Base(dir), err))
			continue
		}
		key := stateKey(dir, local)
		state, err := ws.ReadState(key)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: missing sync baseline; pull first", filepath.Base(dir)))
			continue
		}
		dirty, err := taskHasLocalChanges(local, state, dir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: inspect local changes: %v", taskLabel(local), err))
			continue
		}
		if !dirty {
			continue
		}
		if state.LocalCreated {
			remote, err := createRemoteTask(ctx, client, dir, local, project, catalogIDs)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", filepath.Base(dir), err))
				continue
			}
			newID := stringField(remote, "id")
			if newID == "" {
				problems = append(problems, fmt.Sprintf("%s: task was created remotely but the API response omitted its ID", filepath.Base(dir)))
				continue
			}
			local = workspace.MetadataFromRemote(remote)
			if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), local); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", newID, err))
				continue
			}
			state.Remote = cloneMap(remote)
			state.LocalCreated = false
			state.StagedDiscard = false
			if err := ws.WriteState(newID, state); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", newID, err))
				continue
			}
			if oldPath := ws.StatePathForKey(key); oldPath != ws.StatePathForKey(newID) {
				_ = os.Remove(oldPath)
			}
			key = newID
			newDir := ws.TaskPath(workspace.DirectoryName(remote))
			if _, err := os.Stat(newDir); err == nil && newDir != dir {
				newDir = ws.TaskPath(workspace.DirectoryName(remote) + "-" + safeName(newID))
			}
			if newDir != dir {
				if err := os.Rename(dir, newDir); err != nil {
					fmt.Fprintf(out, "Created task %s, but kept its local folder at %s: %v\n", newID, filepath.Base(dir), err)
				} else {
					dir = newDir
				}
			}
			fmt.Fprintf(out, "Created task %s.\n", taskLabel(local))
		} else {
			id := stringField(local, "id")
			remote, err := client.GetTask(ctx, id)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: fetch current remote task: %v", taskLabel(local), err))
				continue
			}
			if projectID := relationshipID(remote["project"]); projectID != "" && projectID != project.ID {
				problems = append(problems, fmt.Sprintf("%s: task now belongs to project %s", taskLabel(local), projectID))
				continue
			}
			conflicts := taskConflicts(local, state.Remote, remote, dir)
			if len(conflicts) > 0 {
				problems = append(problems, fmt.Sprintf("%s: conflict in %s; use `panda clean %s` then pull to take the remote version, or resolve the files manually", taskLabel(local), strings.Join(conflicts, ", "), taskLabel(local)))
				continue
			}
			noteChanges, err := changedNoteFilenames(dir, state.Remote)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: inspect edited notes: %v", taskLabel(local), err))
				continue
			}
			var myNotes []map[string]any
			if len(noteChanges) > 0 {
				myNotes, err = buildMyNotesPayload(ctx, client, dir, remote, state.Remote)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: edit notes: %v", taskLabel(local), err))
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
				problems = append(problems, fmt.Sprintf("%s: %v", taskLabel(local), err))
				continue
			}
			payload, err = remapTaskCatalogRelationships(payload, catalogIDs)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: map task labels/packages: %v", taskLabel(local), err))
				continue
			}
			if changed {
				remote, err = client.UpdateTask(ctx, id, payload)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: update: %v", taskLabel(local), err))
					continue
				}
			}
			if state.StagedDiscard {
				if stringField(remote, "status") != "discarded" {
					remote, err = client.DiscardTask(ctx, id)
					if err != nil {
						problems = append(problems, fmt.Sprintf("%s: discard: %v", taskLabel(local), err))
						continue
					}
					if stringField(remote, "id") != id || stringField(remote, "status") != "discarded" {
						remote, err = client.GetTask(ctx, id)
						if err != nil {
							problems = append(problems, fmt.Sprintf("%s: verify discard: %v", taskLabel(local), err))
							continue
						}
					}
					if stringField(remote, "status") != "discarded" {
						problems = append(problems, fmt.Sprintf("%s: discard action completed but status is %q", taskLabel(local), stringField(remote, "status")))
						continue
					}
				}
				state.StagedDiscard = false
			}
			if len(myNotes) > 0 {
				remote, err = client.EditMyNotes(ctx, id, myNotes)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: edit notes: %v", taskLabel(local), err))
					continue
				}
				if remote == nil || remote["notes"] == nil {
					remote, err = client.GetTask(ctx, id)
					if err != nil {
						problems = append(problems, fmt.Sprintf("%s: refresh edited notes: %v", taskLabel(local), err))
						continue
					}
				}
				if err := syncRemoteNotes(dir, remote, state.StagedNotes, nil); err != nil {
					problems = append(problems, fmt.Sprintf("%s: refresh edited notes: %v", taskLabel(local), err))
					continue
				}
			}
			state.Remote = cloneMap(remote)
			if err := writeRemoteToLocal(dir, remote); err != nil {
				problems = append(problems, fmt.Sprintf("%s: update local baseline: %v", taskLabel(local), err))
				continue
			}
		}
		state, err = pushStagedNotes(ctx, client, ws, dir, local, key, state)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", taskLabel(local), err))
			continue
		}
		fmt.Fprintf(out, "%s %s.\n", colorText(out, "1;32", "Pushed"), taskLabel(local))
		pushed++
	}
	if len(problems) == 0 {
		if err := pushProjectCatalogDeletes(ctx, client, ws, project, catalogPlans); err != nil {
			problems = append(problems, err.Error())
		}
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
		fmt.Fprintln(out, "No local changes to push.")
	}
	return nil
}

func taskHasLocalChanges(local map[string]any, state workspace.State, dir string) (bool, error) {
	if state.LocalCreated || state.StagedDiscard || len(state.StagedNotes) > 0 {
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
