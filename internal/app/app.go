package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/content"
	"panda-dashboard/internal/solutions"
	"panda-dashboard/internal/workspace"
)

var editableFields = []string{
	"title", "type", "priority", "complexity", "labels", "status", "assignees", "reviewers",
}

func Run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printHelp(out)
		return nil
	}
	if args[0] == "help" {
		if len(args) > 1 && args[1] != "--help" && args[1] != "-h" {
			return printCommandHelp(args[1], out)
		}
		printHelp(out)
		return nil
	}
	if len(args) > 1 && (args[len(args)-1] == "--help" || args[len(args)-1] == "-h") {
		return printCommandHelp(args[0], out)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	client := solutions.NewClient(cfg.APIURL, cfg.Token)
	command, args := args[0], args[1:]
	switch command {
	case "login":
		return login(context.Background(), cfg, client, args, in, out)
	case "config":
		return configCommand(context.Background(), cfg, client, args, out)
	case "completion":
		return completionCommand(context.Background(), cfg, client, args, out)
	case "pull", "diff", "push", "create", "edit", "view", "add-note", "discard", "clean":
		ws, err := projectWorkspace(cfg)
		if err != nil {
			return err
		}
		switch command {
		case "pull":
			if len(args) != 0 {
				return errors.New("usage: panda pull")
			}
			if err := requireLogin(cfg); err != nil {
				return err
			}
			return pull(context.Background(), client, ws, cfg.Project, out)
		case "diff":
			if len(args) != 0 {
				return errors.New("usage: panda diff")
			}
			return diff(ws, out)
		case "push":
			if len(args) != 0 {
				return errors.New("usage: panda push")
			}
			if err := requireLogin(cfg); err != nil {
				return err
			}
			return push(context.Background(), client, ws, cfg.Project, out)
		case "create":
			return createTask(ws, cfg.Project, args, in, out)
		case "edit":
			return editTask(context.Background(), ws, args, client, cfg, in, out)
		case "view":
			return viewTask(ws, args, out)
		case "add-note":
			return addNote(ws, args, in, out)
		case "discard":
			return discard(ws, args, out)
		case "clean":
			return clean(ws, args, in, out)
		}
	default:
		printHelp(errOut)
		return fmt.Errorf("unknown command %q", command)
	}
	return nil
}

func completeLabels(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, out io.Writer) error {
	if len(args) < 1 || len(args) > 2 || args[0] != "labels" {
		return errors.New("usage: panda completion labels [prefix]")
	}
	if cfg.Project == nil {
		return errors.New("no project selected; run `panda config set project <name>` first")
	}
	if err := requireLogin(cfg); err != nil {
		return err
	}
	prefix := ""
	if len(args) == 2 {
		prefix = strings.ToLower(args[1])
	}
	labels, err := client.ListLabels(ctx, cfg.Project.ID)
	if err != nil {
		return err
	}
	for _, label := range labels {
		name := stringField(label, "name")
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			fmt.Fprintln(out, name)
		}
	}
	return nil
}

func completionCommand(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, out io.Writer) error {
	if len(args) == 1 {
		switch args[0] {
		case "bash":
			fmt.Fprintln(out, bashCompletion)
			return nil
		case "zsh":
			fmt.Fprintln(out, zshCompletion)
			return nil
		case "fish":
			fmt.Fprintln(out, fishCompletion)
			return nil
		}
	}
	return completeLabels(ctx, cfg, client, args, out)
}

const bashCompletion = `_panda_complete() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local candidate
  if [[ "${COMP_WORDS[1]}" == "edit" && $COMP_CWORD -eq 3 ]]; then
    COMPREPLY=( $(compgen -W "labels" -- "$cur") )
  elif [[ "${COMP_WORDS[1]}" == "edit" && "${COMP_WORDS[3]}" == "labels" ]]; then
    COMPREPLY=()
    while IFS= read -r candidate; do COMPREPLY+=("$candidate"); done < <(panda completion labels "$cur" 2>/dev/null)
  elif [[ "${COMP_WORDS[1]}" == "completion" && "${COMP_WORDS[2]}" == "labels" ]]; then
    COMPREPLY=()
    while IFS= read -r candidate; do COMPREPLY+=("$candidate"); done < <(panda completion labels "$cur" 2>/dev/null)
  else
    COMPREPLY=( $(compgen -W "login config pull diff push create edit view add-note discard clean completion" -- "$cur") )
  fi
}
complete -F _panda_complete panda`

const zshCompletion = `#compdef panda
_panda() {
  local cur="${words[CURRENT]}"
  if [[ "${words[2]}" == "edit" && $CURRENT -eq 4 ]]; then
    compadd -- labels
  elif [[ "${words[2]}" == "edit" && "${words[4]}" == "labels" ]]; then
    compadd -- "${(@f)$(panda completion labels "$cur" 2>/dev/null)}"
  elif [[ "${words[2]}" == "completion" && "${words[3]}" == "labels" ]]; then
    compadd -- "${(@f)$(panda completion labels "$cur" 2>/dev/null)}"
  else
    compadd -- login config pull diff push create edit view add-note discard clean completion
  fi
}
compdef _panda panda`

const fishCompletion = `complete -c panda -f -n '__fish_seen_subcommand_from edit; and test (count (commandline -opc)) -ge 4; and test (commandline -opc)[4] = labels' -a '(panda completion labels (commandline -ct) 2>/dev/null)'
complete -c panda -f -n '__fish_seen_subcommand_from edit; and test (count (commandline -opc)) -eq 3' -a labels
complete -c panda -f -n '__fish_seen_subcommand_from completion; and test (commandline -opc)[3] = labels' -a '(panda completion labels (commandline -ct) 2>/dev/null)'
complete -c panda -f -n 'not __fish_seen_subcommand_from login config pull diff push create edit view add-note discard clean completion' -a 'login config pull diff push create edit view add-note discard clean completion'`

func printHelp(out io.Writer) {
	fmt.Fprintf(out, "\x1b[1;32mPANDA DASHBOARD\x1b[0m — sync Solutions tasks with local files\n\n")
	fmt.Fprintln(out, `Usage:

  panda <command> [arguments]

Commands:
  login                         Authenticate with Solutions
  config set project <name>     Select the active project
  config get project            Show the active project
  pull                          Fetch project tasks and merge local changes
  diff                          Show local changes pending push
  push                          Apply local changes to Solutions
  create [title]                Create a local task
  edit <task|localId> [labels|json] Edit task fields, labels, or JSON
  view <taskNumber|localId>     Display a task
  add-note <task> [text]        Stage a task note
  discard <task> [note]         Stage task discard and optional note
  clean [taskNumber] [--yes]    Discard local changes (all tasks if omitted)
  completion <shell>             Print bash, zsh, or fish completion setup
  completion labels [prefix]    Print matching project labels

Task files and configuration are stored below the OS user config directory in
panda-dashboard/. Set EDITOR to choose the editor used by edit and note commands.`)
}

func printCommandHelp(command string, out io.Writer) error {
	help := map[string]string{
		"login":      "Usage: panda login [email]\nPrompts for credentials and stores the session token.",
		"config":     "Usage: panda config set project <name> | panda config get project\nSelect or display the active Solutions project.",
		"pull":       "Usage: panda pull\nFetch project tasks, preserve local edits, and report overlapping changes.",
		"diff":       "Usage: panda diff\nShow local edits and staged actions relative to the last-synced baseline.",
		"push":       "Usage: panda push\nCheck for remote conflicts, then apply local edits and staged actions.",
		"create":     "Usage: panda create [title]\nCreate a local task with a temporary ID; push assigns its task number.",
		"edit":       "Usage: panda edit <taskNumber|localId> [labels [labelName|--remove=labelName...]|json]\nInteractively edit title, type, priority, complexity, labels, and description. Type e to open the description in $EDITOR. Use `labels` for label-only editing or `json` to edit all metadata directly.",
		"view":       "Usage: panda view <taskNumber|localId>\nDisplay task metadata and its Markdown description.",
		"add-note":   "Usage: panda add-note <taskNumber|localId> [text]\nStage a Markdown note for the next push.",
		"discard":    "Usage: panda discard <taskNumber|localId> [note]\nStage status=discarded and an optional Markdown note.",
		"clean":      "Usage: panda clean [taskNumber|localId] [--yes]\nDiscard local edits; without a task ID, confirm before cleaning the project.",
		"completion": "Usage: panda completion <bash|zsh|fish> | panda completion labels [prefix]\nInstall shell completion or print matching project labels.",
	}
	text, ok := help[command]
	if !ok {
		return fmt.Errorf("no help available for %q", command)
	}
	fmt.Fprintf(out, "\x1b[1;32m%s\x1b[0m\n\n", strings.ToUpper(command))
	fmt.Fprintln(out, text)
	return nil
}

func login(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, in io.Reader, out io.Writer) error {
	var email string
	if len(args) > 1 {
		return errors.New("usage: panda login [email]")
	}
	if len(args) == 1 {
		email = args[0]
	} else {
		value, err := prompt(in, out, "Email: ")
		if err != nil {
			return err
		}
		email = value
	}
	fmt.Fprint(out, "Password: ")
	var password string
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		data, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return err
		}
		password = string(data)
	} else {
		value, err := prompt(in, io.Discard, "")
		if err != nil {
			return err
		}
		password = value
	}
	token, err := client.Login(ctx, email, password)
	if err != nil {
		return err
	}
	cfg.Token = token
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	fmt.Fprintln(out, "Logged in successfully.")
	return nil
}

func configCommand(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, out io.Writer) error {
	if len(args) == 2 && args[0] == "get" && args[1] == "project" {
		if cfg.Project == nil {
			fmt.Fprintln(out, "No project selected.")
		} else {
			fmt.Fprintf(out, "%s (%s)\n", cfg.Project.Name, cfg.Project.ID)
		}
		return nil
	}
	if len(args) == 3 && args[0] == "set" && args[1] == "project" {
		if cfg.Token == "" {
			return errors.New("not logged in; run `panda login` first")
		}
		project, err := client.FindProject(ctx, args[2])
		if err != nil {
			return err
		}
		cfg.Project = &config.Project{ID: project.ID, Name: project.Name}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Fprintf(out, "Selected project %s (%s).\n", project.Name, project.ID)
		return nil
	}
	return errors.New("usage: panda config get project | panda config set project <name>")
}

func projectWorkspace(cfg *config.Config) (*workspace.Workspace, error) {
	if cfg.Project == nil || cfg.Project.ID == "" || cfg.Project.Name == "" {
		return nil, errors.New("no project selected; run `panda config set project <name>` first")
	}
	root, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		return nil, err
	}
	ws := workspace.New(root)
	if err := ws.Ensure(); err != nil {
		return nil, err
	}
	return ws, nil
}

func requireLogin(cfg *config.Config) error {
	if cfg.Token == "" {
		return errors.New("not logged in; run `panda login` first")
	}
	return nil
}

func pull(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	tasks, err := client.ListTasks(ctx, project.ID)
	if err != nil {
		return err
	}
	updated, created, conflictCount := 0, 0, 0
	remoteIDs := make(map[string]bool, len(tasks))
	for _, remote := range tasks {
		id := stringField(remote, "id")
		if id == "" {
			continue
		}
		remoteIDs[id] = true
		dir, local, err := ws.FindByRemoteID(id)
		if errors.Is(err, os.ErrNotExist) {
			dir = filepath.Join(ws.Root, workspace.DirectoryName(remote))
			if _, statErr := os.Stat(dir); statErr == nil {
				dir = filepath.Join(ws.Root, workspace.DirectoryName(remote)+"-"+id)
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
			if err := syncRemoteNotes(dir, remote, state.StagedNotes); err != nil {
				return err
			}
			if err := ws.WriteState(id, state); err != nil {
				return err
			}
			created++
			continue
		}
		if err != nil {
			return err
		}
		state, err := ws.ReadState(id)
		if err != nil {
			return fmt.Errorf("task %s has no sync baseline; run clean then pull: %w", id, err)
		}
		fields := mergeMetadata(local, state.Remote, remote)
		if len(fields) != 0 {
			conflictCount += len(fields)
			fmt.Fprintf(out, "%s task %s: %s\n", colorText(out, "1;31", "Conflict"), taskLabel(local), strings.Join(fields, ", "))
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
			fmt.Fprintf(out, "%s task %s: description\n", colorText(out, "1;31", "Conflict"), taskLabel(local))
		} else if string(localMarkdown) == baseMarkdown {
			if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(remoteMarkdown), 0o600); err != nil {
				return err
			}
		}
		if err := syncRemoteNotes(dir, remote, state.StagedNotes); err != nil {
			return err
		}
		state.Remote = advanceBaseline(state.Remote, remote, fields, descriptionConflict)
		if err := ws.WriteState(id, state); err != nil {
			return err
		}
		updated++
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
		fmt.Fprintf(out, "%s task %s is no longer returned for this project; kept local files.\n", colorText(out, "1;33", "Warning:"), taskLabel(metadata))
		missingRemote++
	}
	fmt.Fprintf(out, "%s %d task(s): %d new, %d updated", colorText(out, "1;32", "Pulled"), len(tasks), created, updated)
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

func diff(ws *workspace.Workspace, out io.Writer) error {
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	changes := 0
	for _, dir := range dirs {
		local, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		key := stateKey(dir, local)
		state, err := ws.ReadState(key)
		if err != nil || state.LocalCreated {
			fmt.Fprintf(out, "New task %s: %s\n", filepath.Base(dir), stringField(local, "title"))
			changes++
			continue
		}
		for _, field := range editableFields {
			if !same(valueAt(local, field), valueAt(state.Remote, field)) {
				fmt.Fprintf(out, "%s %s: %v -> %v\n", taskLabel(local), field, valueAt(state.Remote, field), valueAt(local, field))
				changes++
			}
		}
		baseMarkdown, err := content.HTMLToMarkdown(stringField(state.Remote, "description"))
		if err != nil {
			return err
		}
		localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
		if err == nil && string(localMarkdown) != baseMarkdown {
			fmt.Fprintf(out, "%s description: modified\n", colorText(out, "1;36", taskLabel(local)))
			changes++
		}
		if len(state.StagedNotes) > 0 {
			fmt.Fprintf(out, "%s notes: %d staged\n", colorText(out, "1;36", taskLabel(local)), len(state.StagedNotes))
			changes += len(state.StagedNotes)
		}
	}
	if changes == 0 {
		fmt.Fprintln(out, "No local changes.")
	}
	return nil
}

func push(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, project *config.Project, out io.Writer) error {
	dirs, err := ws.TaskDirectories()
	if err != nil {
		return err
	}
	var problems []string
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
		if state.LocalCreated {
			remote, err := createRemoteTask(ctx, client, dir, local, project)
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
			if err := ws.WriteState(newID, state); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", newID, err))
				continue
			}
			if oldPath := ws.StatePathForKey(key); oldPath != ws.StatePathForKey(newID) {
				_ = os.Remove(oldPath)
			}
			key = newID
			newDir := filepath.Join(ws.Root, workspace.DirectoryName(remote))
			if _, err := os.Stat(newDir); err == nil && newDir != dir {
				newDir = filepath.Join(ws.Root, workspace.DirectoryName(remote)+"-"+safeName(newID))
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
			payload, changed, err := mergeForPush(local, remote, state.Remote, dir)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", taskLabel(local), err))
				continue
			}
			if changed {
				remote, err = client.UpdateTask(ctx, id, payload)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: update: %v", taskLabel(local), err))
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
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(out, "Error: %s\n", problem)
		}
		return fmt.Errorf("push had %d failure(s)", len(problems))
	}
	if len(dirs) == 0 {
		fmt.Fprintln(out, "No tasks to push.")
	}
	return nil
}

func createRemoteTask(ctx context.Context, client *solutions.Client, dir string, metadata map[string]any, project *config.Project) (map[string]any, error) {
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return nil, err
	}
	html, err := content.MarkdownToHTML(string(description))
	if err != nil {
		return nil, err
	}
	task := cloneMap(metadata)
	task["project"] = map[string]any{"id": project.ID, "label": project.Name}
	task["description"] = html
	delete(task, "id")
	delete(task, "localId")
	delete(task, "number")
	delete(task, "label")
	delete(task, "version")
	delete(task, "notes")
	return client.CreateTask(ctx, task)
}

func mergeForPush(local, remote, baseline map[string]any, dir string) (map[string]any, bool, error) {
	merged := cloneMap(remote)
	changed := false
	for _, field := range editableFields {
		localValue := valueAt(local, field)
		baseValue := valueAt(baseline, field)
		remoteValue := valueAt(remote, field)
		if !same(localValue, baseValue) && !same(localValue, remoteValue) {
			merged[field] = localValue
			changed = true
		}
	}
	baseMarkdown, err := content.HTMLToMarkdown(stringField(baseline, "description"))
	if err != nil {
		return nil, false, err
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return nil, false, err
	}
	if string(localMarkdown) != baseMarkdown {
		html, err := content.MarkdownToHTML(string(localMarkdown))
		if err != nil {
			return nil, false, err
		}
		merged["description"] = html
		changed = true
	}
	return merged, changed, nil
}

func taskConflicts(local, baseline, remote map[string]any, dir string) []string {
	var conflicts []string
	for _, field := range editableFields {
		localValue, baseValue, remoteValue := valueAt(local, field), valueAt(baseline, field), valueAt(remote, field)
		if !same(localValue, baseValue) && !same(remoteValue, baseValue) && !same(localValue, remoteValue) {
			conflicts = append(conflicts, field)
		}
	}
	baseMarkdown, err := content.HTMLToMarkdown(stringField(baseline, "description"))
	if err != nil {
		return append(conflicts, "description")
	}
	localMarkdown, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return append(conflicts, "description")
	}
	if remoteHTML := stringField(remote, "description"); remoteHTML != stringField(baseline, "description") {
		remoteMarkdown, convertErr := content.HTMLToMarkdown(remoteHTML)
		if convertErr != nil || (string(localMarkdown) != baseMarkdown && string(localMarkdown) != remoteMarkdown) {
			conflicts = append(conflicts, "description")
		}
	}
	return conflicts
}

func pullMetadataConflicts(local, baseline, remote map[string]any) []string {
	var conflicts []string
	for _, field := range editableFields {
		localValue, baseValue, remoteValue := valueAt(local, field), valueAt(baseline, field), valueAt(remote, field)
		if !same(localValue, baseValue) && !same(remoteValue, baseValue) && !same(localValue, remoteValue) {
			conflicts = append(conflicts, field)
		}
	}
	return conflicts
}

func mergeMetadata(local, baseline, remote map[string]any) []string {
	return pullMetadataConflicts(local, baseline, remote)
}

func advanceBaseline(baseline, remote map[string]any, conflicts []string, descriptionConflict bool) map[string]any {
	advanced := cloneMap(remote)
	for _, field := range conflicts {
		if value, exists := baseline[field]; exists {
			advanced[field] = value
		} else {
			delete(advanced, field)
		}
	}
	if descriptionConflict {
		advanced["description"] = stringField(baseline, "description")
	}
	return advanced
}

func writeRemoteToLocal(dir string, remote map[string]any) error {
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), workspace.MetadataFromRemote(remote)); err != nil {
		return err
	}
	markdown, err := content.HTMLToMarkdown(stringField(remote, "description"))
	if err != nil {
		return err
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), []byte(markdown), 0o600); err != nil {
		return err
	}
	return nil
}

func pushStagedNotes(ctx context.Context, client *solutions.Client, ws *workspace.Workspace, dir string, local map[string]any, key string, state workspace.State) (workspace.State, error) {
	id := stringField(local, "id")
	for len(state.StagedNotes) > 0 {
		filename := state.StagedNotes[0]
		markdown, err := os.ReadFile(filepath.Join(dir, "notes", filename))
		if err != nil {
			return state, err
		}
		html, err := content.MarkdownToHTML(string(markdown))
		if err != nil {
			return state, err
		}
		if err := client.AddNote(ctx, id, html); err != nil {
			return state, err
		}
		state.StagedNotes = state.StagedNotes[1:]
		if err := ws.WriteState(key, state); err != nil {
			return state, err
		}
		updated, err := client.GetTask(ctx, id)
		if err != nil {
			return state, err
		}
		state.Remote = cloneMap(updated)
		if err := syncRemoteNotes(dir, updated, state.StagedNotes); err != nil {
			return state, err
		}
		_ = os.Remove(filepath.Join(dir, "notes", filename))
		if err := ws.WriteState(key, state); err != nil {
			return state, err
		}
	}
	return state, ws.WriteState(key, state)
}

func syncRemoteNotes(dir string, remote map[string]any, stagedNotes []string) error {
	items, _ := remote["notes"].([]any)
	counts := make(map[string]int)
	staged := make(map[string]bool, len(stagedNotes))
	for _, filename := range stagedNotes {
		staged[filename] = true
	}
	for _, item := range items {
		note, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := stringField(note, "note")
		timestamp := time.Unix(0, 0)
		if raw, ok := note["timestamp"].(float64); ok && raw > 0 {
			timestamp = time.UnixMilli(int64(raw))
		}
		author := "unknown"
		if addedBy, ok := note["addedBy"].(map[string]any); ok && stringField(addedBy, "label") != "" {
			author = stringField(addedBy, "label")
		}
		markdown, err := content.HTMLToMarkdown(text)
		if err != nil {
			return err
		}
		base := noteFilename(timestamp, author, 1)
		counts[base]++
		filename := noteFilename(timestamp, author, counts[base])
		path := filepath.Join(dir, "notes", filename)
		if staged[filename] {
			continue
		}
		if err := workspace.WriteFileAtomic(path, []byte(markdown), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func createTask(ws *workspace.Workspace, project *config.Project, args []string, in io.Reader, out io.Writer) error {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		var err error
		title, err = prompt(in, out, "Task title: ")
		if err != nil {
			return err
		}
	}
	if title == "" {
		return errors.New("task title cannot be empty")
	}
	localID := "local-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dir := filepath.Join(ws.Root, localID)
	metadata := map[string]any{
		"localId":   localID,
		"project":   map[string]any{"id": project.ID, "label": project.Name},
		"title":     title,
		"type":      "feature",
		"priority":  "medium",
		"status":    "new",
		"labels":    []any{},
		"assignees": []any{},
		"reviewers": []any{},
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o700); err != nil {
		return err
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	if err := workspace.WriteFileAtomic(filepath.Join(dir, "description.md"), nil, 0o600); err != nil {
		return err
	}
	if err := ws.WriteState(localID, workspace.State{LocalCreated: true}); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created local task %s. Run `panda edit %s` to edit it and `panda push` to publish it.\n", localID, localID)
	return nil
}

func editTask(ctx context.Context, ws *workspace.Workspace, args []string, client *solutions.Client, cfg *config.Config, in io.Reader, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: panda edit <taskNumber|localId> [labels [labelName...]]")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	if len(args) == 2 {
		if args[1] == "json" {
			if err := openEditor(filepath.Join(dir, "metadata.json")); err != nil {
				return err
			}
			return promptDescriptionEditor(in, out, dir)
		}
		if args[1] != "labels" {
			return errors.New("field selectors are `labels` and `json`")
		}
		if cfg.Project == nil {
			return errors.New("no project selected")
		}
		if err := requireLogin(cfg); err != nil {
			return err
		}
		return editLabels(ctx, client, cfg.Project.ID, dir, metadata, nil, in, out)
	}
	if len(args) > 2 {
		if args[1] != "labels" {
			return errors.New("the only field selector is `labels`")
		}
		if cfg.Project == nil {
			return errors.New("no project selected")
		}
		if err := requireLogin(cfg); err != nil {
			return err
		}
		return editLabels(ctx, client, cfg.Project.ID, dir, metadata, args[2:], in, out)
	}
	if err := editMetadataInteractive(dir, metadata, in, out); err != nil {
		return err
	}
	editLabelsChoice, err := prompt(in, out, "Edit labels using project suggestions? [y/N] ")
	if err != nil {
		return err
	}
	if strings.EqualFold(editLabelsChoice, "y") || strings.EqualFold(editLabelsChoice, "yes") {
		if cfg.Project == nil {
			return errors.New("no project selected")
		}
		if err := requireLogin(cfg); err != nil {
			return err
		}
		metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return err
		}
		if err := editLabels(ctx, client, cfg.Project.ID, dir, metadata, nil, in, out); err != nil {
			return err
		}
	}
	return promptDescriptionEditor(in, out, dir)
}

func promptDescriptionEditor(in io.Reader, out io.Writer, dir string) error {
	choice, err := prompt(in, out, "Type `e` to open description.md in $EDITOR, or press Enter to leave it unchanged: ")
	if err != nil {
		return err
	}
	if strings.EqualFold(choice, "e") {
		return openEditor(filepath.Join(dir, "description.md"))
	}
	return nil
}

func editMetadataInteractive(dir string, metadata map[string]any, in io.Reader, out io.Writer) error {
	title, err := prompt(in, out, fmt.Sprintf("Title [%s]: ", stringField(metadata, "title")))
	if err != nil {
		return err
	}
	if title != "" {
		metadata["title"] = title
	}
	for _, option := range []struct {
		field  string
		values []string
	}{
		{field: "type", values: []string{"feature", "bug", "other", "release"}},
		{field: "priority", values: []string{"hotfix", "high", "medium", "low"}},
		{field: "complexity", values: []string{"xs", "s", "m", "l", "xl", "xxl"}},
	} {
		current := stringField(metadata, option.field)
		fieldTitle := strings.ToUpper(option.field[:1]) + option.field[1:]
		choice, err := prompt(in, out, fmt.Sprintf("%s [%s] (%s): ", fieldTitle, current, strings.Join(option.values, "/")))
		if err != nil {
			return err
		}
		if choice == "-" && option.field == "complexity" {
			delete(metadata, option.field)
			continue
		}
		if choice == "" {
			continue
		}
		if !contains(option.values, choice) {
			return fmt.Errorf("invalid %s %q; choose one of %s", option.field, choice, strings.Join(option.values, ", "))
		}
		metadata[option.field] = choice
	}
	if err := workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	return nil
}

func editLabels(ctx context.Context, client *solutions.Client, projectID, dir string, metadata map[string]any, requested []string, in io.Reader, out io.Writer) error {
	labels, err := client.ListLabels(ctx, projectID)
	if err != nil {
		return err
	}
	sort.Slice(labels, func(i, j int) bool {
		return strings.ToLower(stringField(labels[i], "name")) < strings.ToLower(stringField(labels[j], "name"))
	})
	selected := make(map[string]map[string]any)
	if values, ok := metadata["labels"].([]any); ok {
		for _, value := range values {
			if current, ok := value.(map[string]any); ok {
				id := stringField(current, "id")
				if id != "" {
					selected[id] = map[string]any{"id": id, "label": stringField(current, "label")}
				}
			}
		}
	}
	if len(requested) > 0 {
		for _, name := range requested {
			remove := strings.HasPrefix(name, "--remove=")
			name = strings.TrimPrefix(name, "--remove=")
			var match map[string]any
			for _, label := range labels {
				if strings.EqualFold(stringField(label, "name"), name) {
					match = label
					break
				}
			}
			if match == nil {
				return fmt.Errorf("label %q not found; use `panda completion labels %s` to see suggestions", name, name)
			}
			id := stringField(match, "id")
			if remove {
				delete(selected, id)
			} else {
				selected[id] = map[string]any{"id": id, "label": stringField(match, "name")}
			}
		}
	} else {
		for {
			prefix, err := prompt(in, out, "Search labels by prefix (blank to finish): ")
			if err != nil {
				return err
			}
			if prefix == "" {
				break
			}
			for {
				matches := make([]map[string]any, 0)
				for _, label := range labels {
					if strings.HasPrefix(strings.ToLower(stringField(label, "name")), strings.ToLower(prefix)) {
						matches = append(matches, label)
					}
				}
				if len(matches) == 0 {
					fmt.Fprintln(out, "No labels match that prefix.")
					break
				}
				for i, label := range matches {
					mark := " "
					if _, ok := selected[stringField(label, "id")]; ok {
						mark = "✓"
					}
					fmt.Fprintf(out, "  %s %d) %s\n", mark, i+1, stringField(label, "name"))
				}
				choice, err := prompt(in, out, "Select a number to toggle, or type a narrower prefix: ")
				if err != nil {
					return err
				}
				if choice == "" {
					break
				}
				index, parseErr := strconv.Atoi(choice)
				if parseErr != nil || index < 1 || index > len(matches) {
					prefix = choice
					continue
				}
				label := matches[index-1]
				id := stringField(label, "id")
				if _, exists := selected[id]; exists {
					delete(selected, id)
				} else {
					selected[id] = map[string]any{"id": id, "label": stringField(label, "name")}
				}
				break
			}
		}
	}
	result := make([]any, 0, len(selected))
	for _, label := range selected {
		result = append(result, label)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := result[i].(map[string]any)
		right, _ := result[j].(map[string]any)
		return stringField(left, "label") < stringField(right, "label")
	})
	metadata["labels"] = result
	return workspace.WriteMap(filepath.Join(dir, "metadata.json"), metadata)
}

func viewTask(ws *workspace.Workspace, args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: panda view <taskNumber|localId>")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
		return err
	}
	title := taskLabel(metadata) + " — " + stringField(metadata, "title")
	fmt.Fprintf(out, "%s\n\n", colorText(out, "1;32", "# "+title))
	for _, field := range []string{"type", "status", "priority", "complexity", "labels", "assignees", "reviewers"} {
		if value, exists := metadata[field]; exists {
			fmt.Fprintf(out, "%s %v  \n", colorText(out, "1;36", "**"+field+":**"), value)
		}
	}
	fmt.Fprintln(out, colorText(out, "1;32", "\n## Description"))
	description, err := os.ReadFile(filepath.Join(dir, "description.md"))
	if err != nil {
		return err
	}
	fmt.Fprintln(out, highlightMarkdown(out, string(description)))
	fmt.Fprintln(out, colorText(out, "1;32", "\n## Notes"))
	notesDir := filepath.Join(dir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	state, _ := ws.ReadState(stateKey(dir, metadata))
	staged := make(map[string]bool, len(state.StagedNotes))
	for _, name := range state.StagedNotes {
		staged[name] = true
	}
	printed := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(notesDir, entry.Name()))
		if err != nil {
			return err
		}
		marker := ""
		if staged[entry.Name()] {
			marker = colorText(out, "1;33", " [staged]")
		}
		fmt.Fprintf(out, "%s%s\n", colorText(out, "1;36", "### "+strings.TrimSuffix(entry.Name(), ".md")), marker)
		fmt.Fprintln(out, highlightMarkdown(out, string(body)))
		printed = true
	}
	if !printed {
		fmt.Fprintln(out, "No notes.")
	}
	return nil
}

func addNote(ws *workspace.Workspace, args []string, in io.Reader, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: panda add-note <taskNumber|localId> [text]")
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
		return errors.New("usage: panda discard <taskNumber|localId> [note]")
	}
	dir, metadata, err := ws.FindTask(args[0])
	if err != nil {
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
			return errors.New("usage: panda clean [taskNumber|localId] [--yes]")
		}
	}
	if all && !confirmed {
		answer, err := prompt(in, out, "Discard local changes for every task in this project? [y/N] ")
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
	for _, dir := range dirs {
		metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return err
		}
		key := stateKey(dir, metadata)
		state, err := ws.ReadState(key)
		if err != nil {
			return err
		}
		if state.LocalCreated {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			_ = os.Remove(ws.StatePathForKey(key))
			continue
		}
		if err := writeRemoteToLocal(dir, state.Remote); err != nil {
			return err
		}
		state.StagedNotes = nil
		if err := clearNotes(dir, state.Remote); err != nil {
			return err
		}
		if err := ws.WriteState(key, state); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Cleaned local changes for %d task(s).\n", len(dirs))
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

func prompt(in io.Reader, out io.Writer, message string) (string, error) {
	if message != "" {
		fmt.Fprint(out, message)
	}
	var line strings.Builder
	var one [1]byte
	for {
		n, err := in.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				break
			}
			if one[0] != '\r' {
				line.WriteByte(one[0])
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(line.String()), nil
}

func openEditor(path string) error {
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) == 0 {
		return errors.New("EDITOR is not set")
	}
	args := append(editor[1:], path)
	command := exec.Command(editor[0], args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func editTemporaryNote(in io.Reader) (string, error) {
	dir, err := os.MkdirTemp("", "panda-note-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return "", err
	}
	if file, ok := in.(*os.File); ok && !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("add note text as an argument when running without an interactive terminal")
	}
	if err := openEditor(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

func stateKey(dir string, metadata map[string]any) string {
	if id := stringField(metadata, "id"); id != "" {
		return id
	}
	return filepath.Base(dir)
}

func cloneMap(input map[string]any) map[string]any {
	data, _ := json.Marshal(input)
	var clone map[string]any
	_ = json.Unmarshal(data, &clone)
	if clone == nil {
		clone = make(map[string]any)
	}
	return clone
}

func valueAt(values map[string]any, key string) any {
	value, exists := values[key]
	if !exists {
		return nil
	}
	return value
}

func same(left, right any) bool {
	return reflect.DeepEqual(left, right)
}

func stringField(values map[string]any, key string) string {
	if values == nil || values[key] == nil {
		return ""
	}
	return fmt.Sprint(values[key])
}

func relationshipID(value any) string {
	relationship, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return stringField(relationship, "id")
}

func taskLabel(metadata map[string]any) string {
	if number := metadata["number"]; number != nil {
		return fmt.Sprint(number)
	}
	if id := stringField(metadata, "id"); id != "" {
		return id
	}
	if localID := stringField(metadata, "localId"); localID != "" {
		return localID
	}
	return "local task"
}

func isConflictField(fields []string, field string) bool {
	for _, conflict := range fields {
		if conflict == field {
			return true
		}
	}
	return false
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func safeName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else if r == ' ' {
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func fprintln(out io.Writer) {
	fmt.Fprintln(out)
}

func noteFilename(timestamp time.Time, author string, duplicateIndex int) string {
	suffix := ""
	if duplicateIndex > 1 {
		suffix = fmt.Sprintf("-%d", duplicateIndex)
	}
	return timestamp.UTC().Format("20060102T150405.000Z") + "-" + safeName(author) + suffix + ".md"
}

func uniqueNoteFilename(notesDir string, timestamp time.Time, author string) (string, error) {
	for index := 1; ; index++ {
		filename := noteFilename(timestamp, author, index)
		_, err := os.Stat(filepath.Join(notesDir, filename))
		if errors.Is(err, os.ErrNotExist) {
			return filename, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func colorText(out io.Writer, style, value string) string {
	file, ok := out.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(file.Fd())) {
		return value
	}
	return "\x1b[" + style + "m" + value + "\x1b[0m"
}

func highlightMarkdown(out io.Writer, markdown string) string {
	file, ok := out.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(file.Fd())) {
		return markdown
	}
	bold := regexp.MustCompile(`\*\*(.+?)\*\*`)
	code := regexp.MustCompile("`([^`]+)`")
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = colorText(out, "1;36", line)
			continue
		}
		line = bold.ReplaceAllStringFunc(line, func(match string) string {
			return colorText(out, "1", strings.TrimSuffix(strings.TrimPrefix(match, "**"), "**"))
		})
		line = code.ReplaceAllStringFunc(line, func(match string) string {
			return colorText(out, "36", strings.TrimSuffix(strings.TrimPrefix(match, "`"), "`"))
		})
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
