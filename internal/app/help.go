package app

import (
	"fmt"
	"io"
	"strings"
)

type manualOption struct {
	name        string
	description string
}

type manualPage struct {
	name        string
	synopsis    []string
	description []string
	arguments   []manualOption
	options     []manualOption
	examples    []string
	seeAlso     []string
}

var manualPages = map[string]manualPage{
	"help": {
		name:        "panda help - display command manuals",
		synopsis:    []string{"panda help [command [subcommand ...]]"},
		description: []string{"Display the main command index or the manual page for a command. The same page is available with `panda <command> --help`."},
		examples:    []string{"panda help", "panda help list", "panda help config set project"},
		seeAlso:     []string{"panda(1)"},
	},
	"login": {
		name:        "panda login - authenticate with Solutions",
		synopsis:    []string{"panda login [email]"},
		description: []string{"Sign in to Solutions and save the session token in the local Panda configuration. If email is omitted, Panda prompts for it. The password is always prompted for and is hidden when entered in a terminal."},
		arguments:   []manualOption{{"email", "Solutions account email. If omitted, Panda prompts for it."}},
		examples:    []string{"panda login", "panda login user@example.com"},
		seeAlso:     []string{"panda-config(1)", "panda-pull(1)", "panda-push(1)"},
	},
	"config": {
		name:        "panda config - manage project configuration",
		synopsis:    []string{"panda config set project <name>", "panda config get project"},
		description: []string{"Select a Solutions project or display the currently selected project. Project selection is stored in the local Panda configuration."},
		examples:    []string{"panda config set project \"Project Name\"", "panda config get project"},
		seeAlso:     []string{"panda-login(1)", "panda-path(1)"},
	},
	"config set": {
		name:        "panda config set - set a configuration value",
		synopsis:    []string{"panda config set project <name>"},
		description: []string{"Validate the named Solutions project and make it the active project. Requires a successful login."},
		arguments:   []manualOption{{"name", "Name of the Solutions project to select."}},
		examples:    []string{"panda config set project \"Project Name\""},
		seeAlso:     []string{"panda config get(1)", "panda-login(1)"},
	},
	"config set project": {
		name:        "panda config set project - select the active project",
		synopsis:    []string{"panda config set project <name>"},
		description: []string{"Look up a Solutions project by name and save its ID and name as the active project. Requires a successful login."},
		arguments:   []manualOption{{"name", "Name of the Solutions project to select."}},
		examples:    []string{"panda config set project \"Project Name\""},
		seeAlso:     []string{"panda-config-get-project(1)", "panda-login(1)"},
	},
	"config get": {
		name:        "panda config get - display a configuration value",
		synopsis:    []string{"panda config get project"},
		description: []string{"Print the active project name and ID, or report that no project is selected."},
		examples:    []string{"panda config get project"},
		seeAlso:     []string{"panda config set(1)"},
	},
	"config get project": {
		name:        "panda config get project - display the active project",
		synopsis:    []string{"panda config get project"},
		description: []string{"Print the active project name and ID, or report that no project is selected."},
		examples:    []string{"panda config get project"},
		seeAlso:     []string{"panda-config-set-project(1)"},
	},
	"path": {
		name:        "panda path - print the active workspace path",
		synopsis:    []string{"panda path"},
		description: []string{"Print the selected project's workspace path without creating it. A project must be selected first."},
		examples:    []string{"panda path"},
		seeAlso:     []string{"panda-config(1)"},
	},
	"completion": {
		name:        "panda completion - generate shell integration or find labels",
		synopsis:    []string{"panda completion <bash|zsh|fish>", "panda completion labels [prefix]"},
		description: []string{"Print shell completion code for installation in the selected shell's startup or completion file. The labels subcommand prints matching labels from the active project."},
		arguments: []manualOption{
			{"shell", "One of bash, zsh, or fish."},
			{"prefix", "Optional label-name prefix for the labels subcommand."},
		},
		examples: []string{"panda completion bash", "panda completion labels bug"},
		seeAlso:  []string{"panda-edit(1)"},
	},
	"completion bash": {
		name:        "panda completion bash - print Bash completion code",
		synopsis:    []string{"panda completion bash"},
		description: []string{"Print Bash completion code. Add the output to a Bash completion file and reload the shell."},
		examples:    []string{"panda completion bash"},
	},
	"completion zsh": {
		name:        "panda completion zsh - print Zsh completion code",
		synopsis:    []string{"panda completion zsh"},
		description: []string{"Print Zsh completion code. Add the output to a Zsh completion file and reload the shell."},
		examples:    []string{"panda completion zsh"},
	},
	"completion fish": {
		name:        "panda completion fish - print Fish completion code",
		synopsis:    []string{"panda completion fish"},
		description: []string{"Print Fish completion code. Add the output to a Fish completion file and reload the shell."},
		examples:    []string{"panda completion fish"},
	},
	"completion labels": {
		name:        "panda completion labels - find project labels",
		synopsis:    []string{"panda completion labels [prefix]"},
		description: []string{"Print labels in the active project whose names begin with prefix. When prefix is omitted, print all available labels. Requires a selected project and a successful login."},
		arguments:   []manualOption{{"prefix", "Optional label-name prefix to search for."}},
		examples:    []string{"panda completion labels", "panda completion labels bug"},
		seeAlso:     []string{"panda-edit(1)"},
	},
	"list": {
		name:     "panda list - list local tasks by status",
		synopsis: []string{"panda list [status filters] [other options]"},
		description: []string{
			"List tasks in the active project's local workspace, grouped by workflow status and sorted by rank. The default view includes new, toDo, inProgress, inReview, and completed tasks.",
			"Status selectors are combined as a union. --all includes every status, including statuses not otherwise recognized by Panda. The existing --status selector accepts one or more comma-separated known statuses.",
			"Complexity maps to story points and estimated hours. Each group has a subtotal, followed by a total for all tasks visible after status and content filters are applied.",
		},
		options: []manualOption{
			{"--current", "Show toDo, inProgress, inReview, and completed tasks."},
			{"--staging", "Show staging tasks."},
			{"--released", "Show released tasks."},
			{"--discarded", "Show discarded tasks."},
			{"--all", "Show tasks with every status, including unknown statuses."},
			{"--status <status,...>", "Show the listed comma-separated statuses. Known values: new, toDo, inProgress, inReview, completed, staging, released, discarded."},
			{"--no-complexity", "Show only tasks without a complexity value."},
			{"--no-description", "Show only tasks with a missing or blank local description."},
		},
		examples: []string{
			"panda list",
			"panda list --current",
			"panda list --staging",
			"panda list --status 'new,toDo' --no-complexity",
			"panda list --all --no-description",
		},
		seeAlso: []string{"panda-pull(1)", "panda-view(1)"},
	},
	"pull": {
		name:        "panda pull - synchronize tasks from Solutions",
		synopsis:    []string{"panda pull"},
		description: []string{"Fetch tasks for the active project and merge remote changes into the local workspace. Local edits are preserved; overlapping local and remote changes are reported as conflicts. Requires a selected project and a successful login."},
		examples:    []string{"panda pull"},
		seeAlso:     []string{"panda-diff(1)", "panda-push(1)"},
	},
	"diff": {
		name:        "panda diff - review local changes",
		synopsis:    []string{"panda diff [taskNumber|localId] [options]"},
		description: []string{"Display pending local edits grouped by task. By default, changed fields and a short description/note summary are shown. Terminal output longer than the screen is sent to $PAGER (defaults to less -FRX)."},
		arguments:   []manualOption{{"taskNumber|localId", "Optional task identifier to restrict the diff to one task."}},
		options: []manualOption{
			{"--verbose", "Show changed description and note lines with - and + markers."},
			{"--summary", "Print counts of edited, discarded, and new tasks."},
		},
		examples: []string{"panda diff", "panda diff --verbose", "panda diff 12345 --verbose", "panda diff --summary"},
		seeAlso:  []string{"panda-pull(1)", "panda-push(1)"},
	},
	"push": {
		name:        "panda push - apply local changes to Solutions",
		synopsis:    []string{"panda push"},
		description: []string{"Check for remote conflicts, then send local task edits and staged actions to Solutions. A successful operation updates the local synchronization baseline. Requires a selected project and a successful login."},
		examples:    []string{"panda diff --verbose", "panda push"},
		seeAlso:     []string{"panda-diff(1)", "panda-pull(1)"},
	},
	"create": {
		name:        "panda create - create a local task",
		synopsis:    []string{"panda create [title]"},
		description: []string{"Create a task in the local workspace with a temporary local ID. If title is omitted, Panda prompts for it. A successful push creates the task in Solutions and assigns its task number."},
		arguments:   []manualOption{{"title", "Optional task title; if omitted, Panda prompts for it."}},
		examples:    []string{"panda create", "panda create \"Improve task search\"", "panda push"},
		seeAlso:     []string{"panda-edit(1)", "panda-push(1)"},
	},
	"edit": {
		name:     "panda edit - edit a task's fields and description",
		synopsis: []string{"panda edit <taskNumber|localId>", "panda edit <taskNumber|localId> labels [labelName|--remove=labelName ...]", "panda edit <taskNumber|localId> json"},
		description: []string{
			"Interactively edit a task's title, type, priority, and complexity. Labels may be selected interactively or changed by name. The task description is edited in $EDITOR when requested.",
			"The json selector opens metadata.json for advanced metadata editing and then offers the description editor prompt.",
		},
		arguments: []manualOption{
			{"taskNumber|localId", "Identifier of the task to edit."},
			{"labels", "Open the label picker. Follow with label names to add them or --remove=labelName to remove them."},
			{"json", "Open metadata.json in $EDITOR for advanced metadata editing."},
		},
		examples: []string{"panda edit 12345", "panda edit 12345 labels", "panda edit 12345 labels urgent --remove=bug", "panda edit 12345 json"},
		seeAlso:  []string{"panda-edit-note(1)", "panda-view(1)", "panda-diff(1)"},
	},
	"edit-note": {
		name:        "panda edit-note - edit one of your synced notes",
		synopsis:    []string{"panda edit-note <taskNumber|localId> <note-file>"},
		description: []string{"Open one of the current user's synced task notes in $EDITOR. Use the note filename shown by panda view; the .md extension is optional. Review the local edit with panda diff --verbose and publish it with panda push."},
		arguments: []manualOption{
			{"taskNumber|localId", "Identifier of the task containing the note."},
			{"note-file", "Synced Markdown note filename shown by panda view; .md is optional."},
		},
		examples: []string{"panda edit-note 12345 20261028T120000.000Z-username", "panda diff --verbose", "panda push"},
		seeAlso:  []string{"panda-add-note(1)", "panda-view(1)", "panda-diff(1)"},
	},
	"view": {
		name:        "panda view - display a task",
		synopsis:    []string{"panda view <taskNumber|localId>"},
		description: []string{"Display task metadata, the local Markdown description, and synced notes. Staged notes are marked in the output."},
		arguments:   []manualOption{{"taskNumber|localId", "Identifier of the task to display."}},
		examples:    []string{"panda view 12345"},
		seeAlso:     []string{"panda-edit(1)", "panda-edit-note(1)", "panda-diff(1)"},
	},
	"add-note": {
		name:        "panda add-note - stage a new task note",
		synopsis:    []string{"panda add-note <taskNumber|localId> [text]"},
		description: []string{"Stage a Markdown note for the next push. If text is omitted, open a temporary note in $EDITOR. Empty notes are rejected."},
		arguments: []manualOption{
			{"taskNumber|localId", "Identifier of the task receiving the note."},
			{"text", "Optional note text. If omitted, Panda opens the note editor."},
		},
		examples: []string{"panda add-note 12345", "panda add-note 12345 \"Followed up with the team\"", "panda push"},
		seeAlso:  []string{"panda-edit-note(1)", "panda-push(1)"},
	},
	"discard": {
		name:        "panda discard - stage a task for discard",
		synopsis:    []string{"panda discard <taskNumber|localId> [note]"},
		description: []string{"Set the task's local status to discarded. If note is supplied, stage it as a Markdown note with the status change. The change is sent to Solutions by panda push."},
		arguments: []manualOption{
			{"taskNumber|localId", "Identifier of the task to discard."},
			{"note", "Optional Markdown note to stage with the status change."},
		},
		examples: []string{"panda discard 12345", "panda discard 12345 \"No longer needed\"", "panda push"},
		seeAlso:  []string{"panda-add-note(1)", "panda-push(1)"},
	},
	"clean": {
		name:     "panda clean - discard local task changes",
		synopsis: []string{"panda clean [taskNumber|localId] [--yes]"},
		description: []string{
			"Restore the last-synced local version and discard staged actions for one task. Run panda pull afterward to fetch newer remote changes.",
			"When the task argument is omitted, clean applies to the whole project and asks for confirmation. --yes skips that confirmation.",
		},
		arguments: []manualOption{{"taskNumber|localId", "Optional task identifier. If omitted, clean applies to the entire project."}},
		options:   []manualOption{{"--yes, -y", "Skip confirmation when cleaning the whole project."}},
		examples:  []string{"panda clean 12345", "panda clean --yes"},
		seeAlso:   []string{"panda-diff(1)", "panda-pull(1)"},
	},
}

type mainCommand struct {
	command     string
	synopsis    string
	description string
}

var mainCommands = []mainCommand{
	{"login", "[email]", "Authenticate with Solutions and save a session."},
	{"config set project <name>", "", "Validate and select the active project."},
	{"config get project", "", "Display the active project."},
	{"path", "", "Print the selected project's workspace path."},
	{"completion <bash|zsh|fish>", "", "Print shell completion setup code."},
	{"completion labels [prefix]", "", "Search labels in the active project."},
	{"list", "[status filters] [other filters]", "List tasks with effort totals."},
	{"pull", "", "Fetch tasks and merge remote changes into the workspace."},
	{"diff", "[task] [--verbose] [--summary]", "Review pending local changes."},
	{"push", "", "Publish local edits and staged actions to Solutions."},
	{"create", "[title]", "Create a task locally."},
	{"edit", "<task> [labels ...|json]", "Edit task fields, labels, or metadata."},
	{"edit-note", "<task> <note-file>", "Edit one of your synced notes."},
	{"view", "<task>", "Display task metadata, description, and notes."},
	{"add-note", "<task> [text]", "Stage a new task note."},
	{"discard", "<task> [note]", "Stage a task discard and optional note."},
	{"clean", "[task] [--yes]", "Discard local edits for one task or the project."},
	{"help", "[command ...]", "Display this index or a command manual."},
}

var mainCommandDetails = map[string][]string{
	"list": {
		"Status filters: --current, --staging, --released, --discarded, --all, --status <status,...>.",
		"Other filters: --no-complexity, --no-description.",
	},
}

func printHelp(out io.Writer) {
	printManHeader(out, "PANDA")
	writeManSection(out, "NAME", []string{"panda - synchronize Solutions tasks with a local task workspace"})
	writeManSection(out, "SYNOPSIS", []string{"panda [global options] <command> [arguments]"})
	writeManSection(out, "DESCRIPTION", []string{
		"Panda pulls Solutions tasks into Markdown and JSON files, lets you edit them locally, and publishes selected changes with push.",
		"Use command-specific help for detailed syntax and options: panda <command> --help or panda help <command>.",
	})
	fmt.Fprintln(out, "COMMANDS")
	for _, command := range mainCommands {
		synopsis := "panda " + command.command
		if command.synopsis != "" {
			synopsis += " " + command.synopsis
		}
		writeWrapped(out, "    ", synopsis, 84)
		writeWrapped(out, "        ", command.description, 84)
		for _, detail := range mainCommandDetails[command.command] {
			writeWrapped(out, "        ", detail, 84)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "GLOBAL OPTIONS")
	writeManualOption(out, manualOption{"-h, --help", "Display the main manual. Append --help to a command for its manual page."})
	fmt.Fprintln(out)
	fmt.Fprintln(out, "ENVIRONMENT")
	writeManualOption(out, manualOption{"EDITOR", "Select the editor used by edit, edit-note, and add-note."})
	writeManualOption(out, manualOption{"PAGER", "Select the pager used for long diff output; defaults to less -FRX."})
	fmt.Fprintln(out)
	writeManSection(out, "FILES", []string{"Configuration and task files are stored under the operating system's user config directory in panda-dashboard/. Run panda path to print the active workspace path."})
}

func printCommandHelp(command string, out io.Writer) error {
	command = strings.Join(strings.Fields(command), " ")
	page, ok := manualPages[command]
	if !ok {
		return fmt.Errorf("no help available for %q", command)
	}
	pageTitle := "PANDA-" + strings.ToUpper(strings.ReplaceAll(command, " ", "-"))
	printManHeader(out, pageTitle)
	writeManSection(out, "NAME", []string{page.name})
	writeManSection(out, "SYNOPSIS", page.synopsis)
	writeManSection(out, "DESCRIPTION", page.description)
	if len(page.arguments) > 0 {
		fmt.Fprintln(out, "ARGUMENTS")
		for _, argument := range page.arguments {
			writeManualOption(out, argument)
		}
		fmt.Fprintln(out)
	}
	if len(page.options) > 0 {
		fmt.Fprintln(out, "OPTIONS")
		for _, option := range page.options {
			writeManualOption(out, option)
		}
		fmt.Fprintln(out)
	}
	if len(page.examples) > 0 {
		writeManSection(out, "EXAMPLES", page.examples)
	}
	if len(page.seeAlso) > 0 {
		writeManSection(out, "SEE ALSO", []string{strings.Join(page.seeAlso, ", ")})
	}
	return nil
}

func commandHelpKey(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "config":
		if len(args) > 1 {
			switch args[1] {
			case "get", "set":
				if len(args) > 2 && args[2] == "project" {
					return "config " + args[1] + " project"
				}
				return "config " + args[1]
			}
		}
	case "completion":
		if len(args) > 1 {
			switch args[1] {
			case "labels", "bash", "zsh", "fish":
				return "completion " + args[1]
			}
		}
	}
	return args[0]
}

func printManHeader(out io.Writer, title string) {
	title += "(1)"
	const width = 80
	middle := "Panda Dashboard Manual"
	gap := width - len(title)*2 - len(middle)
	if gap < 2 {
		gap = 2
	}
	leftGap := gap / 2
	rightGap := gap - leftGap
	fmt.Fprintf(out, "\x1b[1;32m%s\x1b[0m%s%s%s\x1b[1;32m%s\x1b[0m\n\n",
		title, strings.Repeat(" ", leftGap), middle, strings.Repeat(" ", rightGap), title)
}

func writeManSection(out io.Writer, section string, paragraphs []string) {
	if len(paragraphs) == 0 {
		return
	}
	fmt.Fprintf(out, "%s\n", section)
	for _, paragraph := range paragraphs {
		writeWrapped(out, "    ", paragraph, 84)
	}
	fmt.Fprintln(out)
}

func writeManualOption(out io.Writer, option manualOption) {
	const optionColumn = 32
	const lineWidth = 84
	descriptionWidth := lineWidth - optionColumn
	lines := wrapWords(option.description, descriptionWidth)
	if len(option.name) < optionColumn-4 {
		fmt.Fprintf(out, "    %-*s%s\n", optionColumn-4, option.name, lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(out, "    %-*s%s\n", optionColumn-4, "", line)
		}
		return
	}
	writeWrapped(out, "    ", option.name, lineWidth)
	writeWrapped(out, "        ", option.description, lineWidth)
}

func writeWrapped(out io.Writer, indent, text string, width int) {
	for _, line := range wrapWords(text, width-len(indent)) {
		fmt.Fprintln(out, indent+line)
	}
}

func wrapWords(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	return append(lines, line)
}
