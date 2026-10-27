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
        state/
      {taskNumber-or-localId}/
        description.md
        metadata.json
        notes/
          {timeStamp}-{authorName}.md
```

The `.panda/state` directory stores the last-synced baseline and remote task
IDs needed for synchronization. A task created locally uses a temporary local
ID until a successful push creates it remotely; then its directory is renamed
to the assigned task number.

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

### `completion labels [prefix]`

- Prints matching labels from the selected project for shell completion
- `panda completion bash|zsh|fish` prints shell integration code; source the
  output from the appropriate shell startup/completion file

### `pull`

- If project is not set throws error
- Fetch all tasks for the selected project from the Solutions app, following
  API pagination
- Compare the fetched tasks with the last-synced baseline
- Apply remote-only changes locally and preserve local-only changes
- Report conflicts when the same field changed both locally and remotely
- Changes remain local until a `push` is performed

### `push`

- If project is not set throws error
- Detects remote changes since the last pull and reports overlapping edits as
  conflicts rather than overwriting them
- Skips tasks with no local edits or staged actions
- Applies local changes and staged actions to the Solutions app
- Updates the local baseline only after successful API operations

### `diff`

- If project is not set throws error
- Displays local changes relative to the last-synced baseline that are pending
  to be applied to the Solutions app

### `list [--status {status,...}] [--no-complexity]`

- Lists tasks in the local project workspace, grouped by status in workflow
  order with separated sections and sorted by the fetched `rank` field
- Status headings are uppercase with camel case separated into words (for
  example, `IN PROGRESS`)
- Run `pull` to refresh local task data and rank values from Solutions
- Discarded tasks are hidden by default; include `discarded` in `--status` to
  display them
- `--no-complexity` shows only tasks without a complexity value
- Table columns are number, priority, type, complexity, and title; release tasks
  are cyan
- Examples: `panda list --status 'new,toDo'` and `panda list --no-complexity`

### `create`

- Creates a task locally

### `discard {taskNumber}`

- Stage a change to the task's status to `discarded`
- Optionally stage a markdown note (to be transformed to HTML) with the change

### `edit {taskNumber}`

- Interactively edit title, type, priority, and complexity
- Type `e` when prompted to open `description.md` in `$EDITOR`
- `edit {taskNumber} labels` opens a prefix-search label picker; labels can also
  be added as names or removed with `--remove={labelName}`
- `edit {taskNumber} json` opens `metadata.json` directly for advanced fields

### `view {taskNumber}`

- Display the ticket information in highlighted markdown

### `add-note {taskNumber}`

- Stage a markdown note locally; it is sent to the Solutions app on `push`

### `clean [taskNumber]`

- Discards local edits and staged actions for one task, or for the whole
  project when no task number is provided
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
