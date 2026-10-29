# Panda Dashboard

Panda Dashboard is a CLI tool written in Go that connects with the Solutions
app REST API to sync the current tasks from the task dashboard and allow the user
to update them using the file system.

## Requirements

- Go 1.26+
- No database
- File system, stored under the operating system's user config directory

## Build and setup

```sh
go install ./cmd/panda
panda login
panda config set project "Project Name"
panda pull
```

The binary is named `panda`. Configuration and synced task files are created
under the OS user config directory.

## Capabilities

- Sync Tasks > Bring all the tasks from solutions to files in local
- Help Command like man pages for every command
- Every HTML field must be fetch and transformed to markdown locally
- Every HTML field locally in markdown must be transformed back to HTML to push
  to the solutions app

## Commands

Read `assets/solutions-openapi.json` for the whole `paths` documentation.
The CLI uses the operating system's user config directory (Go's
`os.UserConfigDir()`), under `panda-dashboard/`, for configuration and the
global workspace.

For example, the directory layout is:

```text
{user-config-dir}/panda-dashboard/
  config.json
  workspaces/
    {projectName}/
      .panda/
        labels.json
        packages.json
        state/
      tasks/
        {taskNumber-or-localId}/
          description.md
          metadata.json
          notes/
            {timeStamp}-{authorName}.md
```

The `.panda/state` directory stores task and project-catalog sync baselines and
the remote task IDs needed for synchronization. A task created locally uses a
temporary local ID until a successful push creates it remotely; then its
directory is renamed to the assigned task number. Legacy task directories at the project root are
moved into `tasks/` when the workspace is next opened; unrelated project files
and directories at the root are left in place.

### `login`

- Read User Input Email and Password
- HTTP POST `/auth/login` `{email,password}`
- Saves the `token` in a local configuration file
- Stores credentials/configuration under the OS config directory with
  user-only permissions where supported
- Use that token as header for the subsequent HTTP requests

### `config set project {projectName}`

- Validates the project exists by querying `/data/projects` by name
- Updates the project id and name in local configuration

### `config get project`

- Prints the current project

### `path`

- Prints the selected project's workspace path without creating it

### `completion <bash|zsh|fish>`

- Prints shell integration code; source the
  output from the appropriate shell startup/completion file

### `pull`

- If project is not set throws error
- Fetch all tasks for the selected project from the Solutions app, following
  API pagination
- Compare the fetched tasks with the last-synced baseline
- Pull project labels and packages into `.panda/labels.json` and
  `.panda/packages.json`, preserving local changes and reporting conflicts
- Skip rewriting tasks whose remote `version` matches the stored baseline
- Shows an in-place progress bar when output is a terminal
- Apply remote-only changes locally and preserve local-only changes
- Report conflicts when the same field changed both locally and remotely
- Changes remain local until a `push` is performed

### `push`

- If project is not set throws error
- Detects remote changes since the last pull and reports overlapping edits as
  conflicts rather than overwriting them
- Skips tasks with no local edits or staged actions
- Applies local changes and staged actions to the Solutions app
- Creates, updates, and deletes staged project labels and packages; task updates
  include their label and package assignments
- Updates the local baseline only after successful API operations

### `diff`

- If project is not set throws error
- Displays pending local changes grouped under each task's number and title
- By default, lists changed fields and marks a changed description as `updated`
- Shows type, priority, and complexity for newly created tasks
- `--verbose` (`-v`) shows changed Markdown description lines, edited note lines,
  and staged note contents with `-` and `+` prefixes
- `--summary` (`-s`) prints counts of edited, discarded, and new tasks instead
  of task details
- `panda diff {taskNumber|localId}` limits the view to one task
- Uses `$PAGER` (defaults to `less -FRX`) for interactive output that exceeds the terminal height; less leaves its output visible after exit. Shorter output and piped output are printed directly

### `list [status filters] [--no-complexity] [--no-description] [--no-package] [--no-labels]`

- Lists tasks in the local project workspace, grouped by status in workflow
  order with separated sections and sorted by the fetched `rank` field
- By default, shows `new`, `toDo`, `inProgress`, `inReview`, and `completed`
  tasks; `staging`, `released`, and `discarded` are hidden
- `--current` shows `toDo`, `inProgress`, `inReview`, and `completed`
- `--staging`, `--released`, and `--discarded` show only tasks with the matching
  status; `--all` includes tasks of every status
- Status filters can be combined, and `--status {status,...}` remains available
- Status headings are uppercase with camel case separated into words (for
  example, `IN PROGRESS`)
- Run `pull` to refresh local task data and rank values from Solutions
- Discarded tasks are hidden by default; display them with `--discarded`,
  `--all`, or `--status discarded`
- `--no-complexity` shows only tasks without a complexity value
- `--no-description` shows only tasks whose local description is missing or blank
- `--no-package` shows only tasks without a package; `--no-labels` shows only
  tasks without labels
- Table columns are number, priority, type, complexity, SP, Hours, Package,
  Labels, and title. Complexity maps to xs=1 SP/4 hours, s=2/8, m=3/12,
  l=5/20, xl=8/32, and xxl=13/52
- Shows a subtotal for each status group and a grand total for all visible tasks;
  tasks without a recognized complexity show `-` for SP and Hours and add zero
  to totals
- Examples: `panda list --current`, `panda list --staging`,
  `panda list --status 'new,toDo' --no-complexity`, and
  `panda list --no-description --no-package --no-labels`

### `labels` and `packages`

- `panda labels` / `panda packages` lists the active project's local catalog
- `panda labels create|edit|delete` manages project labels; `panda packages
  create|edit|delete` manages packages (epics)
- Label descriptions are plain text; package descriptions are Markdown locally
  and convert to/from HTML during synchronization
- Package editing also supports start date and estimation fields
- Catalog changes are local until `panda push`; `panda pull` merges remote
  catalog changes and reports conflicts
- Assign none, one, or multiple labels, and one optional package, with
  `panda edit` or the interactive selectors during `panda create`

### `create`

- Creates a task locally; interactive terminals offer arrow-key selectors for
  type, priority, complexity, labels, and optional package, then an optional
  description editor (`e` opens `$EDITOR`, Enter skips)

### `discard {taskNumber}`

- Stage a change to the task's status to `discarded`
- Optionally stage a markdown note (to be transformed to HTML) with the change

### `edit {taskNumber}`

- Interactively edit title, type, priority, and complexity
- Press `e` by itself when prompted to open `description.md` in `$EDITOR` (no
  Enter required)
- `edit {taskNumber} labels` opens a prefix-search label picker; use `none` to
  clear all labels, or add/remove specific names with `--remove={labelName}`
- `edit {taskNumber} package` selects or clears the task's optional package
- In interactive terminals, use arrow keys and Enter for single-choice fields;
  use Space to toggle labels and Enter to finish the multi-select
- `edit {taskNumber} json` opens `metadata.json` directly for advanced fields

### `view {taskNumber}`

- Display the ticket information in highlighted markdown

### `add-note {taskNumber}`

- Stage a markdown note locally; it is sent to the Solutions app on `push`

### `edit-note {taskNumber} {note-file}`

- Opens one of the current user's synced Markdown notes in `$EDITOR`
- Use `view {taskNumber}` to find note filenames
- `diff --verbose` shows local note edits; `push` sends the complete current-user note list through Solutions' `editMyNotes` action, preserving timestamps and unchanged notes

### `clean [taskNumber]`

- Discards local edits and staged actions for one task, or for the whole
  project when no task number is provided
- Cleaning the whole project also restores the project label and package
  catalogs to their last-synced versions
- Restores the last-synced local version; run `pull` afterward to fetch any
  newer remote changes
- Removes unpushed locally-created tasks when cleaning them by local ID or
  cleaning the whole project
- Requires confirmation when cleaning the whole project

## Tasks

I want to be able to edit these:

- Title (Text)
- Type (Enum)
- Priority (Enum)
- Complexity (Enum)
- Details (HTML)
- Labels (Relationship -> `dev.labels`)

And track these:

- Status
- Assignees
- Reviewers
- Notes

## File System

- Must be intuitive and easy to see the tasks metadata

- `{user-config-dir}/panda-dashboard/workspaces/{projectName}`
  - `.panda/state` (last-synced baselines and remote IDs)
  - `{taskNumber}` or `{localId}` for an unpushed task
    - description.md
    - metadata.json
    - notes
      - `{timeStamp}-{authorName}.md`

## CLI

- Similar to the `gh` tool that is interactive
- For a long body like the description, type `e` to open `$EDITOR`
- Colored output; help-page titles are uppercase, bold, and green
- Shell completion for commands and project labels (bash, zsh, fish)
