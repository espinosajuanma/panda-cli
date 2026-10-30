package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type taskFileReference struct {
	ID          string
	Name        string
	Description string
}

func filesCommand(ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: panda files <taskNumber|draft-N> | upload <taskNumber|draft-N> <path> [--description <text>] | download <taskNumber|draft-N> <file-id|index|name> [--output <path>]")
	}
	switch args[0] {
	case "upload":
		return stageFileUpload(ws, args[1:], out)
	case "download":
		return downloadTaskFile(ws, args[1:], client, out)
	default:
		if len(args) != 1 {
			return errors.New("usage: panda files <taskNumber|draft-N>")
		}
		dir, metadata, err := ws.FindTask(args[0])
		if err != nil {
			return err
		}
		return writePagedOutput(out, func(rendered io.Writer) error {
			return renderTaskFiles(ws, dir, metadata, rendered)
		})
	}
}

func renderTaskFiles(ws *workspace.Workspace, dir string, metadata map[string]any, out io.Writer) error {
	fmt.Fprintf(out, "Files for task %s — %s\n\n", taskLabel(metadata), stringField(metadata, "title"))
	fmt.Fprintln(out, "Attached files:")
	files := taskFileReferences(metadata)
	if len(files) == 0 {
		fmt.Fprintln(out, "  No attached files.")
	} else {
		for index, file := range files {
			fmt.Fprintf(out, "  [%d] %s (id: %s)", index+1, file.Name, file.ID)
			if file.Description != "" {
				fmt.Fprintf(out, " — %s", file.Description)
			}
			fmt.Fprintln(out)
		}
	}

	state, err := ws.ReadState(stateKey(dir, metadata))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(state.StagedFiles) > 0 {
		fmt.Fprintln(out, "\nStaged uploads:")
		for _, staged := range state.StagedFiles {
			status := "[staged]"
			if _, err := os.Stat(staged.Path); err != nil {
				status = "[staged; source missing]"
			}
			name := staged.Name
			if name == "" {
				name = filepath.Base(staged.Path)
			}
			fmt.Fprintf(out, "  %s %s (source: %s)", status, name, staged.Path)
			if staged.Description != "" {
				fmt.Fprintf(out, " — %s", staged.Description)
			}
			fmt.Fprintln(out)
		}
	} else if len(files) > 0 {
		fmt.Fprintln(out)
	}
	return nil
}

func stageFileUpload(ws *workspace.Workspace, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: panda files upload <taskNumber|draft-N> <path> [--description <text>]")
	}
	description := ""
	for index := 2; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--description" || arg == "-d":
			if index+1 >= len(args) {
				return errors.New("--description requires text")
			}
			index++
			description = args[index]
		case strings.HasPrefix(arg, "--description="):
			description = strings.TrimPrefix(arg, "--description=")
		default:
			return fmt.Errorf("unexpected option %q; usage: panda files upload <taskNumber|draft-N> <path> [--description <text>]", arg)
		}
	}
	path, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect upload source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("upload source %q is not a regular file", args[1])
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open upload source: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	state, key, err := loadTaskState(ws, dir, metadata)
	if err != nil {
		return err
	}
	state.StagedFiles = append(state.StagedFiles, workspace.StagedFile{
		Path: path, Name: filepath.Base(path), Description: description,
	})
	if err := ws.WriteState(key, state); err != nil {
		return err
	}
	fmt.Fprintf(out, "Staged file %s for task %s. Run `panda push` to upload and attach it.\n", filepath.Base(path), taskLabel(metadata))
	return nil
}

func downloadTaskFile(ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>]")
	}
	if client == nil || client.Token == "" {
		return errors.New("not logged in; run `panda login` first")
	}
	outputPath := ""
	for index := 2; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--output" || arg == "-o":
			if index+1 >= len(args) {
				return errors.New("--output requires a path")
			}
			index++
			outputPath = args[index]
		case strings.HasPrefix(arg, "--output="):
			outputPath = strings.TrimPrefix(arg, "--output=")
		default:
			return fmt.Errorf("unexpected option %q; usage: panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>]", arg)
		}
	}
	if outputPath == "" && len(args) > 2 {
		for _, arg := range args[2:] {
			if arg == "--output" || arg == "-o" || strings.HasPrefix(arg, "--output=") {
				return errors.New("--output requires a non-empty path")
			}
		}
	}
	_, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	taskID := stringField(metadata, "id")
	if taskID == "" {
		return fmt.Errorf("task %s has not been pushed to Solutions yet", taskLabel(metadata))
	}
	file, err := selectTaskFile(metadata, args[1])
	if err != nil {
		return err
	}
	if file.ID == "" {
		return fmt.Errorf("file %q has no Solutions file ID", args[1])
	}

	var destinationPath string
	var destinationFile *os.File
	temporary := outputPath == ""
	if temporary {
		destinationFile, err = createTaskFileTemp(file.Name)
		if err != nil {
			return err
		}
		destinationPath = destinationFile.Name()
	} else {
		absolute, absErr := filepath.Abs(outputPath)
		if absErr != nil {
			return absErr
		}
		destinationPath = absolute
		parent := filepath.Dir(destinationPath)
		destinationFile, err = os.CreateTemp(parent, ".panda-download-*.tmp")
		if err != nil {
			return fmt.Errorf("create download file: %w", err)
		}
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = destinationFile.Close()
			_ = os.Remove(destinationFile.Name())
		}
	}()
	if err := destinationFile.Chmod(0o600); err != nil {
		return err
	}
	remoteName, downloadErr := client.DownloadTaskFile(context.Background(), taskID, file.ID, destinationFile)
	if downloadErr != nil {
		return downloadErr
	}
	if err := destinationFile.Sync(); err != nil {
		return err
	}
	if err := destinationFile.Close(); err != nil {
		return err
	}
	if !temporary {
		if err := os.Rename(destinationFile.Name(), destinationPath); err != nil {
			return fmt.Errorf("save downloaded file: %w", err)
		}
	}
	cleanup = false
	displayName := remoteName
	if displayName == "" {
		displayName = file.Name
	}
	if displayName == "" {
		displayName = file.ID
	}
	fmt.Fprintf(out, "Downloaded %s to %s\n", displayName, destinationPath)
	return nil
}

func taskFileReferences(metadata map[string]any) []taskFileReference {
	items, _ := metadata["files"].([]any)
	files := make([]taskFileReference, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		file := taskFileReference{Description: stringField(entry, "description")}
		switch value := entry["file"].(type) {
		case map[string]any:
			file.ID = stringField(value, "id")
			file.Name = stringField(value, "name")
		case string:
			file.ID = value
		}
		if file.ID == "" {
			file.ID = stringField(entry, "fileId")
		}
		if file.Name == "" {
			file.Name = stringField(entry, "name")
		}
		if file.Name == "" {
			file.Name = "(unnamed file)"
		}
		files = append(files, file)
	}
	return files
}

func selectTaskFile(metadata map[string]any, selector string) (taskFileReference, error) {
	files := taskFileReferences(metadata)
	for _, file := range files {
		if file.ID != "" && file.ID == selector {
			return file, nil
		}
	}
	if index, err := strconv.Atoi(selector); err == nil && index > 0 && index <= len(files) {
		return files[index-1], nil
	}
	var matches []taskFileReference
	for _, file := range files {
		if strings.EqualFold(file.Name, selector) {
			matches = append(matches, file)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return taskFileReference{}, fmt.Errorf("file name %q is ambiguous; use its file ID or list index", selector)
	}
	return taskFileReference{}, fmt.Errorf("file %q was not found on task %s", selector, taskLabel(metadata))
}

func createTaskFileTemp(name string) (*os.File, error) {
	name = stripFileSizeSuffix(name)
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(filepath.Base(name), extension)
	base = safeTempName(base)
	extension = safeTempName(strings.TrimPrefix(extension, "."))
	if extension != "" {
		extension = "." + extension
	}
	if base == "" {
		base = "download"
	}
	return os.CreateTemp("", "panda-"+base+"-*"+extension)
}

func stripFileSizeSuffix(name string) string {
	open := strings.LastIndex(name, " (")
	if open < 0 || !strings.HasSuffix(name, ")") {
		return name
	}
	suffix := name[open+2 : len(name)-1]
	if !strings.HasSuffix(suffix, "B") {
		return name
	}
	hasDigit := false
	for _, char := range suffix {
		if unicode.IsDigit(char) {
			hasDigit = true
			continue
		}
		if char == '.' || unicode.IsLetter(char) {
			continue
		}
		return name
	}
	if hasDigit {
		return name[:open]
	}
	return name
}

func pushStagedFiles(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, metadata map[string]any, key string, state workspace.State) (workspace.State, error) {
	if len(state.StagedFiles) == 0 {
		return state, nil
	}
	taskID := stringField(metadata, "id")
	if taskID == "" {
		return state, fmt.Errorf("task %s has not been created in Solutions yet", taskLabel(metadata))
	}
	if err := validateStagedFiles(state.StagedFiles); err != nil {
		return state, err
	}

	attachments := make([]solutions.FileAttachment, 0, len(state.StagedFiles))
	for _, staged := range state.StagedFiles {
		file, err := os.Open(staged.Path)
		if err != nil {
			return state, fmt.Errorf("open staged file %q: %w", staged.Name, err)
		}
		filename := staged.Name
		if filename == "" {
			filename = filepath.Base(staged.Path)
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
			FileID: link.FileID, Description: staged.Description,
		})
	}

	updated, err := client.AttachFiles(ctx, taskID, attachments)
	if err != nil {
		return state, fmt.Errorf("attach uploaded files to task: %w", err)
	}
	if stringField(updated, "id") != taskID || !containsAttachedFiles(updated, attachments) {
		updated, err = client.GetTask(ctx, taskID)
		if err != nil {
			return state, fmt.Errorf("refresh task after attaching files: %w", err)
		}
	}
	if stringField(updated, "id") != taskID {
		return state, fmt.Errorf("file attachment response did not include task %s", taskID)
	}
	if !containsAttachedFiles(updated, attachments) {
		return state, fmt.Errorf("task %s response did not include the newly attached file references", taskID)
	}
	if err := writeRemoteToLocal(dir, updated); err != nil {
		return state, fmt.Errorf("save attached file references: %w", err)
	}
	state.Remote = cloneMap(updated)
	state.StagedFiles = nil
	if err := ws.WriteState(key, state); err != nil {
		return state, err
	}
	return state, nil
}

func validateStagedFiles(files []workspace.StagedFile) error {
	for _, staged := range files {
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

func safeTempName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' || char == '.':
			builder.WriteRune(char)
		case unicode.IsSpace(char):
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), ".-_")
}

func containsAttachedFiles(taskRecord map[string]any, attachments []solutions.FileAttachment) bool {
	attached := make(map[string]bool)
	for _, file := range taskFileReferences(taskRecord) {
		attached[file.ID] = true
	}
	for _, file := range attachments {
		if !attached[file.FileID] {
			return false
		}
	}
	return true
}
