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

	"github.com/espinosajuanma/panda-cli/internal/fsutil"
	"github.com/espinosajuanma/panda-cli/internal/report"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	syncpkg "github.com/espinosajuanma/panda-cli/internal/sync"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type taskFileReference = syncpkg.FileReference

func filesCommand(ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	return filesCommandContext(context.Background(), ws, args, client, out)
}

func filesCommandContext(ctx context.Context, ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: panda files <taskNumber|draft-N> | upload <taskNumber|draft-N> <path> [--description <text>] | download <taskNumber|draft-N> <file-id|index|name> [--output <path>] [--force]")
	}
	switch args[0] {
	case "upload":
		return stageFileUpload(ws, args[1:], out)
	case "download":
		return downloadTaskFileContext(ctx, ws, args[1:], client, out)
	default:
		if err := suggestionError("files subcommand", args[0], []string{"upload", "download"}); err != nil {
			return err
		}
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
	view := report.TaskFilesView{
		Label: taskLabel(metadata),
		Title: stringField(metadata, "title"),
	}
	for _, file := range taskFileReferences(metadata) {
		view.Files = append(view.Files, report.TaskFile{Name: file.Name, ID: file.ID, Description: file.Description})
	}
	state, err := ws.ReadState(stateKey(dir, metadata))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, staged := range state.StagedFiles {
		name := staged.Name
		if name == "" {
			name = filepath.Base(staged.Path)
		}
		_, statErr := os.Stat(staged.Path)
		view.Staged = append(view.Staged, report.StagedTaskFile{
			Name: name, Path: staged.Path, Description: staged.Description, SourceMissing: statErr != nil,
		})
	}
	return report.RenderTaskFiles(view, out)
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

func downloadTaskFileContext(ctx context.Context, ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>] [--force]")
	}
	if client == nil || client.Token == "" {
		return errors.New("not logged in; run `panda login` first")
	}
	outputPath := ""
	force := false
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
		case arg == "--force", arg == "-f":
			force = true
		default:
			return fmt.Errorf("unexpected option %q; usage: panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>] [--force]", arg)
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
		if _, statErr := os.Lstat(destinationPath); statErr == nil && !force {
			return fmt.Errorf("output file %q already exists; use --force to replace it", destinationPath)
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect output path: %w", statErr)
		}
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
	remoteName, downloadErr := client.DownloadTaskFile(ctx, taskID, file.ID, destinationFile)
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
		if err := fsutil.Replace(destinationFile.Name(), destinationPath); err != nil {
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
	return syncpkg.TaskFileReferences(metadata)
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

func safeTempName(value string) string {
	return workspace.SafeFileComponent(value)
}
