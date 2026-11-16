package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/task"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type FileReference struct {
	ID          string
	Name        string
	Description string
}

func TaskFileReferences(metadata map[string]any) []FileReference {
	items, _ := metadata["files"].([]any)
	files := make([]FileReference, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		file := FileReference{Description: jsonutil.StringField(entry, "description")}
		switch value := entry["file"].(type) {
		case map[string]any:
			file.ID = jsonutil.StringField(value, "id")
			file.Name = jsonutil.StringField(value, "name")
		case string:
			file.ID = value
		}
		if file.ID == "" {
			file.ID = jsonutil.StringField(entry, "fileId")
		}
		if file.Name == "" {
			file.Name = jsonutil.StringField(entry, "name")
		}
		if file.Name == "" {
			file.Name = "(unnamed file)"
		}
		files = append(files, file)
	}
	return files
}

func PushStagedFiles(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, metadata map[string]any, key string, state workspace.State) (workspace.State, error) {
	if len(state.StagedFiles) == 0 {
		return state, nil
	}
	taskID := jsonutil.StringField(metadata, "id")
	if taskID == "" {
		return state, fmt.Errorf("task %s has not been created in Solutions yet", task.Label(metadata))
	}
	if err := ValidateStagedFiles(state.StagedFiles); err != nil {
		return state, err
	}

	attachments := make([]solutions.FileAttachment, 0, len(state.StagedFiles))
	preexistingUploadIDs := false
	for index := range state.StagedFiles {
		staged := state.StagedFiles[index]
		filename := staged.Name
		if filename == "" && staged.Path != "" {
			filename = filepath.Base(staged.Path)
		}
		if staged.RemoteID != "" {
			preexistingUploadIDs = true
			attachments = append(attachments, solutions.FileAttachment{
				FileID: staged.RemoteID, Name: filename, Description: staged.Description,
			})
			continue
		}
		file, err := os.Open(staged.Path)
		if err != nil {
			return state, fmt.Errorf("open staged file %q: %w", staged.Name, err)
		}
		link, uploadErr := client.UploadFile(ctx, filename, file)
		closeErr := file.Close()
		if uploadErr != nil {
			return state, fmt.Errorf("upload staged file %q: %w", staged.Name, uploadErr)
		}
		if closeErr != nil {
			return state, fmt.Errorf("close staged file %q: %w", staged.Name, closeErr)
		}
		attachments = append(attachments, solutions.FileAttachment{
			FileID: link.FileID, Name: filename, Description: staged.Description,
		})
		state.StagedFiles[index].RemoteID = link.FileID
		if err := ws.WriteState(key, state); err != nil {
			return state, fmt.Errorf("save uploaded file ID for %q: %w", staged.Name, err)
		}
	}

	var updated map[string]any
	var err error
	if preexistingUploadIDs {
		updated, err = client.GetTask(ctx, taskID)
		if err != nil {
			return state, fmt.Errorf("check staged file attachments: %w", err)
		}
	}
	pending := missingFileAttachments(updated, attachments)
	if len(pending) > 0 {
		updated, err = client.AttachFiles(ctx, taskID, pending)
		if err != nil {
			return state, fmt.Errorf("attach uploaded files to task: %w", err)
		}
	}
	if updated == nil || jsonutil.StringField(updated, "id") != taskID || !containsAttachedFiles(updated, attachments) {
		updated, err = client.GetTask(ctx, taskID)
		if err != nil {
			return state, fmt.Errorf("refresh task after attaching files: %w", err)
		}
	}
	if jsonutil.StringField(updated, "id") != taskID {
		return state, fmt.Errorf("file attachment response did not include task %s", taskID)
	}
	if !containsAttachedFiles(updated, attachments) {
		return state, fmt.Errorf("task %s response did not include the newly attached file references", taskID)
	}
	if err := WriteRemoteToLocal(dir, updated); err != nil {
		return state, fmt.Errorf("save attached file references: %w", err)
	}
	state.Remote = jsonutil.CloneMap(updated)
	state.StagedFiles = nil
	if err := ws.WriteState(key, state); err != nil {
		return state, err
	}
	return state, nil
}

func missingFileAttachments(taskRecord map[string]any, attachments []solutions.FileAttachment) []solutions.FileAttachment {
	attached := make(map[string]bool)
	for _, file := range TaskFileReferences(taskRecord) {
		attached[file.ID] = true
	}
	pending := make([]solutions.FileAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if !attached[attachment.FileID] {
			pending = append(pending, attachment)
		}
	}
	return pending
}

func ValidateStagedFiles(files []workspace.StagedFile) error {
	for _, staged := range files {
		if staged.RemoteID != "" {
			continue
		}
		info, err := os.Stat(staged.Path)
		if err != nil {
			return fmt.Errorf("staged file %q is unavailable: %w", staged.Name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged file %q is no longer a regular file", staged.Name)
		}
		file, err := os.Open(staged.Path)
		if err != nil {
			return fmt.Errorf("staged file %q cannot be opened: %w", staged.Name, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close staged file %q: %w", staged.Name, err)
		}
	}
	return nil
}

func containsAttachedFiles(taskRecord map[string]any, attachments []solutions.FileAttachment) bool {
	attached := make(map[string]bool)
	for _, file := range TaskFileReferences(taskRecord) {
		attached[file.ID] = true
	}
	for _, file := range attachments {
		if !attached[file.FileID] {
			return false
		}
	}
	return true
}
