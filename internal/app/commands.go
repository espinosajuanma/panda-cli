package app

import "strings"

type mainCommand struct {
	command     string
	synopsis    string
	description string
	workspace   bool
}

// mainCommands is the source for both the help index and root command completion.
var mainCommands = []mainCommand{
	{"login", "[email]", "Authenticate with Solutions and save a session.", false},
	{"config set project <name>", "", "Validate and select the active project.", false},
	{"config get project", "", "Display the active project.", false},
	{"config set project-view <viewId>", "", "Configure the view ID used in canonical task links.", false},
	{"config get project-view", "", "Display the configured task view ID.", false},
	{"path", "", "Print the selected project's workspace path.", false},
	{"completion <bash|zsh|fish>", "", "Print shell completion setup code.", false},
	{"labels [list|create|edit|delete]", "", "Manage project labels locally.", true},
	{"packages [list|create|edit|delete]", "", "Manage project packages (epics) locally.", true},
	{"list", "[status filters] [other filters]", "List tasks with package, labels, and effort totals.", true},
	{"pull", "", "Fetch project tasks, catalogs, and members into the workspace.", true},
	{"diff", "[task] [--verbose|-v] [--summary|-s]", "Review pending local changes.", true},
	{"push", "", "Publish local edits and staged actions to Solutions.", true},
	{"create", "[title] [task field options]", "Create a task locally with interactive or noninteractive options.", true},
	{"edit", "<task> [task field options|labels ...|package ...|assignees ...|json]", "Edit task fields interactively or with safe partial options.", true},
	{"assign", "<task> <teamLead|developer|qa|none>", "Assign eligible project members to a task by role.", true},
	{"link", "<task>", "Print a canonical Markdown link to a verified synced task.", true},
	{"edit-note", "<task> <note-file>", "Edit one of your synced notes.", true},
	{"view", "<task>", "Display task metadata, description, and notes.", true},
	{"add-note", "<task> [text]", "Stage a new task note.", true},
	{"discard", "<task> [note]", "Stage a task discard and optional note.", true},
	{"clean", "[task] [--yes]", "Discard local edits for one task or the project.", true},
	{"help", "[command ...]", "Display this index or a command manual.", false},
}

func rootCommandNames() []string {
	seen := make(map[string]bool, len(mainCommands))
	var names []string
	for _, command := range mainCommands {
		fields := strings.Fields(command.command)
		if len(fields) == 0 || fields[0] == "help" || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		names = append(names, fields[0])
	}
	return names
}

func rootCommandWords() string { return strings.Join(rootCommandNames(), " ") }

func commandNeedsWorkspace(name string) bool {
	for _, command := range mainCommands {
		fields := strings.Fields(command.command)
		if len(fields) > 0 && fields[0] == name && command.workspace {
			return true
		}
	}
	return false
}
