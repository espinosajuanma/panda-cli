package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func editNote(ws *workspace.Workspace, args []string, client *solutions.Client, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: panda edit-note <taskNumber|draft-N> <note-file>")
	}
	if client.Email == "" {
		return errors.New("your login email is not saved; run `panda login` to enable editing your notes")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	state, err := ws.ReadState(stateKey(dir, metadata))
	if err != nil {
		return err
	}
	user, err := client.FindUser(context.Background(), client.Email)
	if err != nil {
		return err
	}
	filename := filepath.Base(args[1])
	if filename != args[1] {
		return errors.New("note-file must be an existing Markdown filename from the task's notes directory")
	}
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}
	remoteNotes, err := getRemoteNotes(state.Remote)
	if err != nil {
		return err
	}
	owned := false
	for _, note := range remoteNotes {
		if note.filename == filename && noteOwnedBy(note, user, client.Email) {
			owned = true
			break
		}
	}
	if !owned {
		return fmt.Errorf("note %q was not found among your synced notes for task %s", filename, taskLabel(metadata))
	}
	path := filepath.Join(dir, "notes", filename)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open note %q: %w", filename, err)
	}
	if err := openEditor(path); err != nil {
		return err
	}
	fmt.Fprintf(out, "Edited %s. Review with `panda diff --verbose`; run `panda push` to apply it.\n", filepath.ToSlash(filepath.Join("notes", filename)))
	return nil
}

func addNote(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: panda add-note <taskNumber|draft-N> [text]")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	text := strings.TrimSpace(strings.Join(args[1:], " "))
	if text == "" {
		text, err = editTemporaryNote(in)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("note cannot be empty")
	}
	return stageNote(ws, dir, metadata, text, out)
}

func discard(ws *workspace.Workspace, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: panda discard <taskNumber|draft-N> [note]")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	key := stateKey(dir, metadata)
	state, err := ws.ReadState(key)
	if err != nil {
		return err
	}
	state.StagedDiscard = true
	if err := ws.WriteState(key, state); err != nil {
		return err
	}
	metadata["status"] = "discarded"
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	if len(args) > 1 {
		return stageNote(ws, dir, metadata, strings.Join(args[1:], " "), out)
	}
	fmt.Fprintf(out, "Staged discard of %s. Run `panda push` to apply it.\n", taskLabel(metadata))
	return nil
}

func stageNote(ws *workspace.Workspace, dir string, metadata map[string]any, text string, out io.Writer) error {
	key := stateKey(dir, metadata)
	state, err := ws.ReadState(key)
	if err != nil {
		return err
	}
	stamp := time.Now().UTC()
	filename, err := uniqueNoteFilename(filepath.Join(dir, "notes"), stamp, "local")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "notes", filename)
	if err := workspace.WriteFileAtomic(path, []byte(text), 0o600); err != nil {
		return err
	}
	state.StagedNotes = append(state.StagedNotes, filename)
	if err := ws.WriteState(key, state); err != nil {
		return err
	}
	fmt.Fprintf(out, "Staged note for %s. Run `panda push` to apply it.\n", taskLabel(metadata))
	return nil
}

func clean(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	all := true
	identifier := ""
	confirmed := false
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			confirmed = true
		} else if identifier == "" {
			identifier = arg
			all = false
		} else {
			return errors.New("usage: panda clean [taskNumber|draft-N] [--yes]")
		}
	}
	if all && !confirmed {
		answer, err := prompt(in, out, "Discard local changes for every task and project catalog? [y/N] ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	if !all {
		dir, _, err := ws.FindTask(identifier)
		if err != nil {
			return err
		}
		dirs = []string{dir}
	}
	cleaned := 0
	var problems []string
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			if all {
				problems = append(problems, fmt.Sprintf("%s: read metadata: %v", filepath.Base(dir), err))
				continue
			}
			return err
		}
		state, key, err := loadTaskState(ws, dir, metadata)
		if err != nil {
			if all {
				problems = append(problems, err.Error())
				continue
			}
			return err
		}
		if state.LocalCreated {
			if err := os.RemoveAll(dir); err != nil {
				if all {
					problems = append(problems, fmt.Sprintf("%s: remove local draft: %v", taskLabel(metadata), err))
					continue
				}
				return err
			}
			removeTaskStateFiles(ws, dir, metadata)
			cleaned++
			continue
		}
		if err := writeRemoteToLocal(dir, state.Remote); err != nil {
			if all {
				problems = append(problems, fmt.Sprintf("%s: restore task: %v", taskLabel(metadata), err))
				continue
			}
			return err
		}
		state.StagedNotes = nil
		state.StagedDiscard = false
		if err := clearNotes(dir, state.Remote); err != nil {
			if all {
				problems = append(problems, fmt.Sprintf("%s: clear notes: %v", taskLabel(metadata), err))
				continue
			}
			return err
		}
		if err := syncRemoteNotes(dir, state.Remote, nil, nil); err != nil {
			if all {
				problems = append(problems, fmt.Sprintf("%s: restore notes: %v", taskLabel(metadata), err))
				continue
			}
			return err
		}
		if err := ws.WriteState(key, state); err != nil {
			if all {
				problems = append(problems, fmt.Sprintf("%s: save sync state: %v", taskLabel(metadata), err))
				continue
			}
			return err
		}
		cleaned++
	}
	if all {
		for _, kind := range []catalogKind{labelCatalog, packageCatalog} {
			baseline, err := readCatalogBaseline(ws, kind)
			if err != nil {
				return err
			}
			if err := writeCatalog(ws, kind, baseline); err != nil {
				return err
			}
		}
	}
	if all {
		fmt.Fprintf(out, "Cleaned local changes for %d task(s) and project catalogs.\n", cleaned)
	} else {
		fmt.Fprintf(out, "Cleaned local changes for %d task(s).\n", cleaned)
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(out, "Error: %s\n", problem)
		}
		return fmt.Errorf("clean had %d failure(s)", len(problems))
	}
	return nil
}

func clearNotes(dir string, remote map[string]any) error {
	notesDir := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	keep := make(map[string]bool)
	counts := make(map[string]int)
	items, _ := remote["notes"].([]any)
	for _, item := range items {
		note, ok := item.(map[string]any)
		if !ok {
			continue
		}
		timestamp := time.Unix(0, 0)
		if raw, ok := note["timestamp"].(float64); ok && raw > 0 {
			timestamp = time.UnixMilli(int64(raw))
		}
		author := "unknown"
		if addedBy, ok := note["addedBy"].(map[string]any); ok && stringField(addedBy, "label") != "" {
			author = stringField(addedBy, "label")
		}
		base := noteFilename(timestamp, author, 1)
		counts[base]++
		keep[noteFilename(timestamp, author, counts[base])] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() && !keep[entry.Name()] {
			if err := os.Remove(filepath.Join(notesDir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
