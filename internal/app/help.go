package app

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

func taskFieldOptionsManual() []manualOption {
	return []manualOption{
		{"--title <text>", "Set or replace the title. Noninteractive create requires this option or positional title text."},
		{"--type <value>", "Set one of feature, bug, other, or release."},
		{"--priority <value>", "Set one of hotfix, high, medium, or low."},
		{"--complexity <value|none>", "Set xs, s, m, l, xl, or xxl; none clears the field."},
		{"--package <name|none>", "Set a package by local catalog name or ID, or clear it with none."},
		{"--assignee <member>", "Set an eligible synced member by name, email, or ID. Repeat for multiple assignees; none clears all."},
		{"--label, --add-label <name|none>", "Add a label from the local project catalog. Repeat to add multiple labels; use none by itself to clear all labels."},
		{"--remove-label <name>", "Remove a label from the task. Repeat to remove multiple labels."},
		{"--description-file <path|->", "Read UTF-8 Markdown from a file, or read it from stdin when the path is -. Empty descriptions are rejected when this option is used."},
	}
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
		description: []string{"Select a Solutions project or display the active project. Configuration is stored locally. Task links use Panda's shared Solutions task view."},
		examples:    []string{"panda config set project \"Project Name\"", "panda config get project"},
		seeAlso:     []string{"panda-login(1)", "panda-path(1)"},
	},
	"config set": {
		name:        "panda config set - set a configuration value",
		synopsis:    []string{"panda config set project <name>"},
		description: []string{"Select the active Solutions project."},
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
		description: []string{"Print the active project name and ID."},
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
		name:        "panda completion - generate shell integration",
		synopsis:    []string{"panda completion <bash|zsh|fish>"},
		description: []string{"Print shell completion code for installation in the selected shell's startup or completion file."},
		arguments: []manualOption{
			{"shell", "One of bash, zsh, or fish."},
		},
		examples: []string{"panda completion bash"},
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
	"labels": {
		name:        "panda labels - manage project labels locally",
		synopsis:    []string{"panda labels [list]", "panda labels create [name] [description]", "panda labels edit <name|id>", "panda labels delete <name|id>"},
		description: []string{"List and manage the active project's labels in the local workspace. Label descriptions are plain text. Create, edit, and delete operations are staged locally and synchronized by panda pull and panda push."},
		examples:    []string{"panda labels", "panda labels create security \"Security review\"", "panda labels edit security", "panda labels delete security", "panda push"},
		seeAlso:     []string{"panda-edit(1)", "panda-pull(1)", "panda-push(1)"},
	},
	"labels create": {
		name:        "panda labels create - create a local project label",
		synopsis:    []string{"panda labels create [name] [description]"},
		description: []string{"Stage a project label locally. If name is omitted, Panda prompts for the name and optional description. The label is created in Solutions on the next push."},
		arguments:   []manualOption{{"name", "Optional label name; prompted for when omitted."}, {"description", "Optional label description."}},
		examples:    []string{"panda labels create security", "panda labels create security \"Security review\"", "panda push"},
	},
	"labels edit": {
		name:        "panda labels edit - edit a local project label",
		synopsis:    []string{"panda labels edit <name|id>"},
		description: []string{"Prompt for a label's name and description. Press Enter to keep a value or enter - to clear the description. Changes remain local until panda push."},
		arguments:   []manualOption{{"name|id", "Label name, remote ID, or local ID."}},
		examples:    []string{"panda labels edit security", "panda push"},
	},
	"labels delete": {
		name:        "panda labels delete - delete a local project label",
		synopsis:    []string{"panda labels delete <name|id>"},
		description: []string{"Stage a label deletion locally and remove that label from local task assignments. The remote label is deleted on push, after affected task updates."},
		arguments:   []manualOption{{"name|id", "Label name, remote ID, or local ID."}},
		examples:    []string{"panda labels delete security", "panda diff", "panda push"},
	},
	"packages": {
		name:        "panda packages - manage project packages locally",
		synopsis:    []string{"panda packages [list]", "panda packages create [title] [description]", "panda packages edit <title|id>", "panda packages delete <title|id>"},
		description: []string{"List and manage the active project's packages (epics) in the local workspace. Package descriptions are Markdown locally and converted to/from HTML during synchronization. Package edits also support start date and estimation. Create, edit, and delete operations are staged locally and synchronized by panda pull and panda push."},
		examples:    []string{"panda packages", "panda packages create \"Billing migration\"", "panda packages edit \"Billing migration\"", "panda packages delete \"Billing migration\"", "panda push"},
		seeAlso:     []string{"panda-edit(1)", "panda-pull(1)", "panda-push(1)"},
	},
	"packages create": {
		name:        "panda packages create - create a local project package",
		synopsis:    []string{"panda packages create [title] [description]"},
		description: []string{"Stage a package locally. If title is omitted, Panda prompts for the title and optional description. The package is created in Solutions on the next push."},
		arguments:   []manualOption{{"title", "Optional package title; prompted for when omitted."}, {"description", "Optional package description."}},
		examples:    []string{"panda packages create \"Billing migration\"", "panda push"},
	},
	"packages edit": {
		name:        "panda packages edit - edit a local project package",
		synopsis:    []string{"panda packages edit <title|id>"},
		description: []string{"Prompt for a package title, description, start date, and estimation. Press Enter to keep a value or enter - to clear an optional value. Changes remain local until panda push."},
		arguments:   []manualOption{{"title|id", "Package title, remote ID, or local ID."}},
		examples:    []string{"panda packages edit \"Billing migration\"", "panda push"},
	},
	"packages delete": {
		name:        "panda packages delete - delete a local project package",
		synopsis:    []string{"panda packages delete <title|id>"},
		description: []string{"Stage a package deletion locally and clear it from local task assignments. The remote package is deleted on push, after affected task updates."},
		arguments:   []manualOption{{"title|id", "Package title, remote ID, or local ID."}},
		examples:    []string{"panda packages delete \"Billing migration\"", "panda diff", "panda push"},
	},
	"list": {
		name:     "panda list - list local tasks by status",
		synopsis: []string{"panda list [status filters] [other options]"},
		description: []string{
			"List tasks in the active project's local workspace, grouped by workflow status and sorted by rank. The default view includes new, toDo, inProgress, inReview, and completed tasks.",
			"Status selectors are combined as a union. --all includes every status, including statuses not otherwise recognized by Panda. The existing --status selector accepts one or more comma-separated known statuses.",
			"Complexity maps to story points and estimated hours. Package and label names are shown with each task. Each group has a subtotal, followed by a total for all tasks visible after status and content filters are applied.",
			"Long terminal output uses $PAGER (default less -FRX) and remains visible after leaving the pager; piped output is printed directly.",
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
			{"--no-package", "Show only tasks that are not assigned to a package."},
			{"--no-labels", "Show only tasks that have no labels assigned."},
		},
		examples: []string{
			"panda list",
			"panda list --current",
			"panda list --staging",
			"panda list --status 'new,toDo' --no-complexity",
			"panda list --no-package --no-labels",
		},
		seeAlso: []string{"panda-pull(1)", "panda-view(1)"},
	},
	"estimate": {
		name:     "panda estimate - assign complexity to local tasks",
		synopsis: []string{"panda estimate [status[,status...]] [--type <type,...>] [--all]"},
		description: []string{
			"Interactively assign complexity to local tasks in the selected statuses and types. The default status is new. By default, feature, bug, and other tasks are eligible; release tasks are excluded. Tasks that already have a complexity are also skipped unless --all is supplied.",
			"On terminals, choose a task's complexity with the arrow-key selector. Press Escape to return to the task actions, d to view its Markdown description, s to skip it, or q to stop. Complexity choices include story points and estimated hours.",
			"Changes are saved locally to task metadata and remain local until panda push. In non-terminal use, enter a complexity value, d to view the description, q to stop, or leave the value empty to skip.",
		},
		arguments: []manualOption{{"status[,status...]", "Optional comma-separated or space-separated workflow statuses. Known values: new, toDo, inProgress, inReview, completed, staging, released, discarded. Defaults to new."}},
		options: []manualOption{
			{"--type <type,...>", "Select task types with a comma-separated list: feature, bug, other, or release. Repeat the option to add types. Defaults to feature, bug, and other; include release explicitly to estimate release tasks."},
			{"--all", "Include tasks that already have a complexity so they can be re-rated."},
		},
		examples: []string{"panda estimate", "panda estimate toDo", "panda estimate new,toDo", "panda estimate --type feature,bug", "panda estimate new --all", "panda push"},
		seeAlso:  []string{"panda-list(1)", "panda-edit(1)", "panda-push(1)"},
	},
	"pull": {
		name:        "panda pull - synchronize project data from Solutions",
		synopsis:    []string{"panda pull"},
		description: []string{"Fetch tasks, labels, packages, and project members for the active project, then merge task and catalog changes into the local workspace. Members are stored in .panda/members.json for assignment commands. Local edits are preserved; overlapping task or catalog changes are reported as conflicts. Requires a selected project and a successful login."},
		examples:    []string{"panda pull"},
		seeAlso:     []string{"panda-diff(1)", "panda-push(1)"},
	},
	"diff": {
		name:        "panda diff - review local changes",
		synopsis:    []string{"panda diff [taskNumber|draft-N] [--verbose|-v] [--summary|-s]"},
		description: []string{"Display pending local edits grouped by task. By default, changed fields and a short description/note summary are shown. Terminal output longer than the screen is sent to $PAGER (defaults to less -FRX); less keeps its output visible after exit."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Optional task identifier to restrict the diff to one task. Legacy local-* IDs are also accepted."}},
		options: []manualOption{
			{"--verbose, -v", "Show changed description and note lines with - and + markers."},
			{"--summary, -s", "Print counts of edited, discarded, and new tasks."},
		},
		examples: []string{"panda diff", "panda diff --verbose", "panda diff -v", "panda diff 12345 -v", "panda diff -s"},
		seeAlso:  []string{"panda-pull(1)", "panda-push(1)"},
	},
	"push": {
		name:        "panda push - apply local changes to Solutions",
		synopsis:    []string{"panda push [taskNumber|draft-N ...]"},
		description: []string{"Check for remote conflicts, then send local task edits and staged actions to Solutions. Without selectors, push processes every task; with selectors, it processes only those tasks. Pending project catalogs are still synchronized. Local drafts can be pushed without a sync-state file; synced tasks require a baseline. Requires a selected project and a successful login."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Optional task selectors; repeat to push only those tasks. Legacy local-* IDs are also accepted."}},
		examples:    []string{"panda diff --verbose", "panda push", "panda push draft-1", "panda push draft-1 draft-3"},
		seeAlso:     []string{"panda-diff(1)", "panda-pull(1)"},
	},
	"create": {
		name:        "panda create - create a local task",
		synopsis:    []string{"panda create [title]", "panda create --title <text> [task field options]"},
		description: []string{"Create a task in the local workspace with a draft-N ID. Existing interactive selectors remain available. Supplying task field options selects noninteractive mode; title is required and no prompts or editor are opened. Changes remain local until an explicit panda push."},
		arguments:   []manualOption{{"title", "Optional positional title; noninteractive option mode requires a title and does not prompt."}},
		options:     taskFieldOptionsManual(),
		examples:    []string{"panda create", "panda create --title \"Improve task search\" --type feature --assignee \"Ada Lovelace\" --description-file description.md", "panda create --title \"Scripted task\" --description-file - < description.md"},
		seeAlso:     []string{"panda-edit(1)", "panda-push(1)"},
	},
	"edit": {
		name:     "panda edit - edit a task's fields and description",
		synopsis: []string{"panda edit <taskNumber|draft-N>", "panda edit <taskNumber|draft-N> [task field options]", "panda edit <taskNumber|draft-N> labels [labelName|--remove=labelName ...|none]", "panda edit <taskNumber|draft-N> package [name|none]", "panda edit <taskNumber|draft-N> assignees [member ...|none]", "panda edit <taskNumber|draft-N> json"},
		description: []string{
			"Interactively edit a task's title, type, priority, and complexity. On terminals, use arrow keys and Enter for single-choice fields; use Space to toggle labels and assignees and Enter to save each multi-select. Assign at most one optional package.",
			"For scripts, pass any supported field options to apply only the supplied fields. Option-driven edits do not prompt or open an editor, preserve omitted metadata, and remain local until panda push.",
			"Press e (without Enter) to open the task description in $EDITOR when prompted.",
			"The json selector opens metadata.json for advanced metadata editing and then offers the description editor prompt.",
		},
		arguments: []manualOption{
			{"taskNumber|draft-N", "Identifier of the task to edit. Legacy local-* IDs are also accepted."},
			{"labels", "Open the label picker. Follow with label names to add them, --remove=labelName to remove them, or none to clear all labels."},
			{"package", "Select one package by name or enter none to clear the assignment."},
			{"assignees", "Open the multi-select assignee picker, or follow with one or more project member names, emails, or IDs; none clears all assignees."},
			{"json", "Open metadata.json in $EDITOR for advanced metadata editing."},
		},
		options:  taskFieldOptionsManual(),
		examples: []string{"panda edit 12345", "panda edit 12345 --priority high", "panda edit 12345 --label urgent --remove-label bug", "panda edit 12345 --description-file description.md", "panda edit 12345 labels urgent --remove=bug", "panda edit 12345 json"},
		seeAlso:  []string{"panda-edit-note(1)", "panda-view(1)", "panda-diff(1)"},
	},
	"assign": {
		name:        "panda assign - assign project members by role",
		synopsis:    []string{"panda assign <taskNumber|draft-N> <role[,role...]|none>"},
		description: []string{"Assign every synced project member matching one or more eligible team roles to a task. The role list can be space- or comma-separated; eligible roles are teamLead, developer, and qa. Use none to clear all assignees. Run panda pull first to sync the project team. Changes are local until panda push."},
		arguments: []manualOption{
			{"taskNumber|draft-N", "Identifier of the task to assign. Legacy local-* IDs are also accepted."},
			{"role", "One or more eligible project team roles: teamLead, developer, or qa."},
		},
		examples: []string{"panda assign 12345 developer", "panda assign 12345 teamLead,qa", "panda assign 12345 none", "panda push"},
		seeAlso:  []string{"panda-pull(1)", "panda-edit(1)", "panda-push(1)"},
	},
	"edit-note": {
		name:        "panda edit-note - edit one of your synced notes",
		synopsis:    []string{"panda edit-note <taskNumber|draft-N> <note-file>"},
		description: []string{"Open one of the current user's synced task notes in $EDITOR. Use the note filename shown by panda view; the .md extension is optional. Review the local edit with panda diff --verbose and publish it with panda push."},
		arguments: []manualOption{
			{"taskNumber|draft-N", "Identifier of the task containing the note. Legacy local-* IDs are also accepted."},
			{"note-file", "Synced Markdown note filename shown by panda view; .md is optional."},
		},
		examples: []string{"panda edit-note 12345 20261028T120000.000Z-username", "panda diff --verbose", "panda push"},
		seeAlso:  []string{"panda-add-note(1)", "panda-view(1)", "panda-diff(1)"},
	},
	"view": {
		name:        "panda view - display a task",
		synopsis:    []string{"panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--json]"},
		description: []string{"Display task metadata in an aligned table, plus the local Markdown description, attached file references, and synced notes. Creator/date and sync context are shown when available. Staged notes and file uploads are marked. --no-description and --no-notes omit those sections; --json emits structured output without paging."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Identifier of the task to display. Legacy local-* IDs are also accepted."}},
		options:     []manualOption{{"--no-description", "Omit the Markdown description section."}, {"--no-notes", "Omit the notes section."}, {"--json", "Emit structured JSON instead of the formatted view."}},
		examples:    []string{"panda view 12345", "panda view 12345 --no-notes", "panda view 12345 --json"},
		seeAlso:     []string{"panda-edit(1)", "panda-edit-note(1)", "panda-diff(1)"},
	},
	"files": {
		name:        "panda files - list, stage, and download task files",
		synopsis:    []string{"panda files <taskNumber|draft-N>", "panda files upload <taskNumber|draft-N> <path> [--description <text>]", "panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>]"},
		description: []string{"List file references stored with a task, stage a local file for upload, or download an attached file on demand. Pull stores only file references; it never downloads file content. Uploads are sent and attached by panda push. Downloads are written to an OS temporary file unless --output is supplied."},
		examples:    []string{"panda files 12345", "panda files upload 12345 screenshot.png --description \"Login screen\"", "panda push", "panda files download 12345 1", "panda files download 12345 file-id --output ./screenshot.png"},
		seeAlso:     []string{"panda-pull(1)", "panda-push(1)", "panda-view(1)"},
	},
	"files upload": {
		name:        "panda files upload - stage a task file upload",
		synopsis:    []string{"panda files upload <taskNumber|draft-N> <path> [--description <text>]"},
		description: []string{"Record the absolute source path and optional description in the task's sync state. The file is not copied or uploaded until panda push runs, so keep the source file available until then. After a successful push, Panda keeps only the remote file reference in task metadata."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Task that will receive the file. Local drafts are supported."}, {"path", "Path to a regular local file. The file must still exist when panda push runs."}},
		options:     []manualOption{{"--description <text>, -d <text>", "Optional description saved with the attached file."}},
		examples:    []string{"panda files upload 12345 screenshot.png", "panda files upload draft-1 report.pdf --description \"Test report\"", "panda diff", "panda push"},
		seeAlso:     []string{"panda-files(1)", "panda-push(1)", "panda-diff(1)"},
	},
	"files download": {
		name:        "panda files download - download an attached task file",
		synopsis:    []string{"panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>]"},
		description: []string{"Fetch an attached file only when requested. Select it by file ID, 1-based list index, or an unambiguous name. By default, the binary is written to an OS temporary file and its path is printed. Requires a successful login."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Task containing the attached file."}, {"file-id|index|name", "File identifier, 1-based position from panda files, or unambiguous file name."}},
		options:     []manualOption{{"--output <path>, -o <path>", "Write to this path instead of an OS temporary file."}},
		examples:    []string{"panda files 12345", "panda files download 12345 1", "panda files download 12345 file-id --output ./report.pdf"},
		seeAlso:     []string{"panda-files(1)", "panda-login(1)"},
	},
	"link": {
		name:        "panda link - print a canonical Solutions task link",
		synopsis:    []string{"panda link <taskNumber|draft-N>"},
		description: []string{"Print a Markdown link using Panda's shared Solutions task view and the task ID verified by its local sync baseline. Local, unsynced tasks and tasks without a verified ID cannot be linked."},
		arguments:   []manualOption{{"taskNumber|draft-N", "Identifier of the synced task. Legacy local-* IDs are also accepted."}},
		examples:    []string{"panda link 12345", "[#12345](https://solutions.slingrs.io/prod/runtime/#views/680b8e41805d3740e97cfecb/readOnly/task-id)"},
		seeAlso:     []string{"panda-view(1)"},
	},
	"add-note": {
		name:        "panda add-note - stage a new task note",
		synopsis:    []string{"panda add-note <taskNumber|draft-N> [text]"},
		description: []string{"Stage a Markdown note for the next push. If text is omitted, open a temporary note in $EDITOR. Empty notes are rejected."},
		arguments: []manualOption{
			{"taskNumber|draft-N", "Identifier of the task receiving the note. Legacy local-* IDs are also accepted."},
			{"text", "Optional note text. If omitted, Panda opens the note editor."},
		},
		examples: []string{"panda add-note 12345", "panda add-note 12345 \"Followed up with the team\"", "panda push"},
		seeAlso:  []string{"panda-edit-note(1)", "panda-push(1)"},
	},
	"discard": {
		name:        "panda discard - stage a task for discard",
		synopsis:    []string{"panda discard <taskNumber|draft-N> [note]"},
		description: []string{"Set the task's local status to discarded. If note is supplied, stage it as a Markdown note with the status change. The change is sent to Solutions by panda push."},
		arguments: []manualOption{
			{"taskNumber|draft-N", "Identifier of the task to discard. Legacy local-* IDs are also accepted."},
			{"note", "Optional Markdown note to stage with the status change."},
		},
		examples: []string{"panda discard 12345", "panda discard 12345 \"No longer needed\"", "panda push"},
		seeAlso:  []string{"panda-add-note(1)", "panda-push(1)"},
	},
	"clean": {
		name:     "panda clean - discard local task changes",
		synopsis: []string{"panda clean [taskNumber|draft-N] [--yes]"},
		description: []string{
			"Restore the last-synced local version and discard staged actions for one task. Run panda pull afterward to fetch newer remote changes.",
			"Locally created drafts can be removed even if their sync-state file is missing. Synced tasks without a baseline require panda pull before they can be restored.",
			"When the task argument is omitted, clean restores all recoverable tasks and the project catalogs, asks for confirmation, and reports tasks with missing baselines. --yes skips confirmation.",
		},
		arguments: []manualOption{{"taskNumber|draft-N", "Optional task identifier. If omitted, clean applies to the entire project. Legacy local-* IDs are also accepted."}},
		options:   []manualOption{{"--yes, -y", "Skip confirmation when cleaning the whole project."}},
		examples:  []string{"panda clean 12345", "panda clean --yes"},
		seeAlso:   []string{"panda-diff(1)", "panda-pull(1)"},
	},
}
