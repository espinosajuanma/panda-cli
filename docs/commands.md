# Command reference

Run `panda --help` for the command index or `panda help <command>` for a
command's manual page. The same manual is available with
`panda <command> --help`.

## Setup and configuration

### `panda login [email]`

Authenticates with Solutions and saves the session token in Panda's private
user configuration. If the email is omitted, Panda prompts for it. The password
is prompted for and hidden when entered in a terminal.

### `panda logout`

Removes the saved Solutions token and login email from local configuration.

### `panda config set project <name>`

Looks up a Solutions project by name and selects it as the active project.
Requires a successful login.

### `panda config get project`

Prints the active project name and ID.

### `panda config set api-url <url>` / `panda config get api-url`

Sets or prints the absolute http(s) Solutions API base URL. The
`PANDA_API_URL` environment variable overrides the saved value. Canonical task
links use the configured host and runtime path.

### `panda path`

Prints the active project's workspace path without creating it.

### `panda doctor [--json]`

Runs read-only local diagnostics for configuration, workspace directories and
permissions, lock state, task metadata and baselines, catalogs, project members,
staged files/notes, and leftover atomic-write temporary files. It makes no
network requests and does not repair or create files.

- `OK` findings indicate checks that passed.
- `WARN` findings identify issues that may limit a workflow; warnings do not
  cause a non-zero exit status.
- `FAIL` findings indicate invalid or missing required local data and cause a
  non-zero exit status.
- `--json` emits a versioned document with stable finding codes, levels, paths,
  messages, and summary counts.

```sh
panda doctor
panda doctor --json
```

The JSON document has `schemaVersion: 1`, an `ok` boolean, `summary` counts for
checks/failures/warnings, and a `findings` array. Missing project selection,
session, members, or an uninitialized workspace are warnings; corrupt metadata
and missing baselines for synced tasks are failures.

```json
{
  "schemaVersion": 1,
  "ok": true,
  "summary": {"checks": 12, "failures": 0, "warnings": 0},
  "findings": [
    {"level": "ok", "code": "workspace.available", "path": "/home/user/.config/panda-dashboard/workspaces/Project", "message": "project workspace is available"}
  ]
}
```

### `panda completion <bash|zsh|fish>`

Prints shell completion code. Add it to the appropriate shell startup or
completion file, then reload the shell.

## Synchronization

### `panda pull`

Fetches tasks, labels, packages, and project members, following API pagination.
Panda merges remote updates with the local workspace, preserves independent
local changes, and reports overlapping changes as conflicts. Project members
and roles are saved locally for assignment commands. Tasks whose remote version
has not changed are not rewritten.

Changes remain local until `panda push`.
Task file references are stored in metadata, but file content is not downloaded
during pull.

### `panda push [taskNumber|draft-N ...] [--json]`

Checks for remote changes before sending local task and catalog edits. It skips
unchanged tasks, applies staged notes, file uploads, and discards. Baselines
advance after successful API operations. Overlapping edits are reported instead
of overwritten. With no selectors it processes every task; with one or more
selectors it processes only those tasks. Pending project label/package changes
are still synchronized, since those catalogs are shared by the project.
All task selectors are validated before any API operation begins.
Local drafts can be pushed without a sync-state file; synced tasks need a
baseline, so run `panda pull` first if one is missing.

`--json` emits a versioned report with per-task outcomes (`created`, `pushed`,
`skipped`, `conflict`, or `failed`), attempted actions, catalog create/update/
delete counts, errors, and a summary. The report is written to stdout; failures
are still returned as a non-zero exit status.

```sh
panda push
panda push draft-1
panda push draft-1 draft-3
panda push 12345
panda push --json
panda push draft-1 --json
```

Push JSON uses `schemaVersion: 1`. Each task result includes a selector, remote
ID/number when available, outcome, actions (`create`, `fields`, `discard`,
`editedNotes`, `stagedNotes`, `files`), and an optional message. `remoteCreated`
indicates that the API created a task even if later local persistence or staged
actions failed. Task-level and catalog failures appear in `errors`;
`summary.failed` counts those failures and `summary.pushed` counts fully completed
task pushes.

```json
{
  "schemaVersion": 1,
  "tasks": [{"selector": "draft-1", "id": "task-42", "number": 42, "title": "Improve search", "remoteCreated": true, "outcome": "created", "actions": ["create"]}],
  "catalogs": {
    "labels": {"changed": false, "created": 0, "updated": 0, "deleted": 0},
    "packages": {"changed": false, "created": 0, "updated": 0, "deleted": 0}
  },
  "errors": [],
  "summary": {"pushed": 1, "created": 1, "updated": 0, "skipped": 0, "failed": 0}
}
```

### `panda diff [taskNumber|draft-N] [--verbose|-v] [--summary|-s] [--json]`

Shows pending task and project-catalog changes. An optional task identifier
limits output to one task.

- Default output summarizes changed fields and descriptions.
- `--verbose` shows changed Markdown description and note lines with `-` and `+`
  prefixes, and shows staged file upload paths.
- `--summary` prints counts of edited, discarded, and new tasks.
- `--json`: emit a versioned structured report of task and catalog changes,
  including summary counts. Combine with `--verbose` to include Markdown line
  changes. `--summary` is redundant and cannot be combined with `--json`.
- Long terminal output uses `$PAGER`, defaulting to `less -FRX`; leaving the
  pager keeps its output visible. Piped output is printed directly.

Diff JSON uses `schemaVersion: 1`. Task changes identify fields, description and
note edits, staged notes, and staged files. Verbose Markdown changes include
`lines` entries with `marker` and `text`; non-verbose output omits line details.
JSON is written directly without paging or color.

```json
{
  "schemaVersion": 1,
  "tasks": [{"task": {"number": 42, "title": "Improve search"}, "new": false, "discarded": false, "changes": [{"kind": "field", "field": "priority", "before": "medium", "after": "high"}]}],
  "catalogs": {"labels": [], "packages": []},
  "summary": {"edited": 1, "discarded": 0, "new": 0, "labels": 0, "packages": 0, "changeCount": 1}
}
```

## Tasks

### `panda estimate [status[,status...]] [--type <type,...>] [--all]`

Interactively assign a complexity to local tasks in the selected workflow
statuses and types. The default status is `new`; pass one or more
comma-separated or space-separated statuses to process another group. Statuses
are iterated in workflow order, and tasks within each status are sorted by rank.

By default, tasks of type `feature`, `bug`, and `other` are eligible; `release`
tasks are excluded. Use `--type <type,...>` to select types explicitly. Valid
types are `feature`, `bug`, `other`, and `release`; repeated `--type` options
combine. Explicitly selecting `release` includes release tasks.

By default, tasks that already have a complexity are skipped. `--all` also
includes those tasks for re-rating. On a terminal, use the arrow keys and Enter
to choose a complexity; every option shows its story points and estimated hours.
Press Escape while choosing to return to the task actions. Press `d` to view a
task's description, `s` to skip it, or `q` to stop early.
In non-terminal mode, enter `xs`, `s`, `m`, `l`, `xl`, or `xxl`; enter `d` to
view the description, `q` to stop, or leave the choice blank to skip.

Complexity choices map as follows: `xs` is 1 SP/4 hours, `s` is 2/8, `m` is
3/12, `l` is 5/20, `xl` is 8/32, and `xxl` is 13/52. Each selection is saved
locally; run `panda push` to publish the changes.

```sh
panda estimate
panda estimate toDo
panda estimate new,toDo
panda estimate --type feature,bug
panda estimate --type bug --type other
panda estimate new --all
```

### `panda list [status filters] [assignee/reviewer filters] [other filters] [--wide] [--json]`

Lists local tasks grouped in workflow order and sorted by their fetched rank.
The default statuses are `new`, `toDo`, `inProgress`, `inReview`, and
`completed`; `staging`, `released`, and `discarded` are hidden by default.
Run `panda pull` to refresh local task data and rank values.

On terminals, Panda hides lower-priority columns as needed to fit the available
width and wraps long titles. `--wide` forces the full table. Piped output always
uses the full table.

Status filters can be combined:

- `--current`: `toDo`, `inProgress`, `inReview`, and `completed`.
- `--staging`, `--released`, `--discarded`: select one status.
- `--all`: include all statuses, including unknown ones.
- `--status <status,...>`: select comma-separated known statuses.

Content filters:

- `--no-complexity`: tasks without a complexity value.
- `--no-description`: tasks with a missing or blank local description.
- `--no-package`: tasks without a package.
- `--no-labels`: tasks without labels.

Relationship filters:

- `--assignee <member,...>`: select tasks assigned to any listed member. Repeat
  the option or pass comma-separated names, emails, or IDs.
- `--reviewer <member,...>`: select tasks reviewed by any listed member, with the
  same repeatable/comma-separated behavior.
- `me` selects the member matching the saved login email and requires synced
  project members. `none` selects tasks with no assignees/reviewers and must be
  used alone for that filter.
- Multiple values within one filter use OR; when both `--assignee` and
  `--reviewer` are supplied, the task must match both filters.
- `--wide`: show every table column, even when the table is wider than the
  terminal. Cannot be combined with `--json`.
- `--json`: emit a versioned JSON document containing the filtered groups, task
  metadata, display names for relationships, per-task effort estimates, and
  group/project totals, and per-assignee effort totals. JSON output is written
  directly without paging or color.

Complexity totals map `xs` to 1 SP/4 hours, `s` to 2/8, `m` to 3/12, `l` to
5/20, `xl` to 8/32, and `xxl` to 13/52. Unknown values contribute zero and
display dashes.

The text summary after the task groups lists visible-task counts, story points,
and hours per assignee. Shared-task effort is counted in full for each assigned
person; totals may therefore exceed the project total.

Long terminal output uses `$PAGER` (default `less -FRX`) and remains visible
after leaving the pager. Piped output is printed directly.

```sh
panda list --current
panda list --staging
panda list --status 'new,toDo' --no-complexity
panda list --no-description --no-package --no-labels
panda list --assignee me --reviewer none
panda list --assignee 'Ada Lovelace,Grace Hopper'
panda list --assignee ada@example.com --reviewer grace@example.com
panda list --current --wide
panda list --current --json
```

The JSON document has `schemaVersion: 1`. Each task includes its original
metadata under `task`, plus display-oriented `package`, `labels`, `assignees`,
and `reviewers` values. `storyPoints` and `hours` are `null` when the task's
complexity is unknown. Group and overall totals include task counts, story
points, and hours. `assigneeTotals` includes visible-task counts and full
SP/hour totals per assignee, plus an `Unassigned` entry when appropriate.

```json
{
  "schemaVersion": 1,
  "groups": [
    {
      "status": "new",
      "tasks": [{"task": {"number": 42, "title": "Improve search"}, "storyPoints": 3, "hours": 12, "package": null, "labels": [], "assignees": ["Ada Lovelace"], "reviewers": []}],
      "totals": {"tasks": 1, "storyPoints": 3, "hours": 12}
    }
  ],
  "totals": {"tasks": 1, "storyPoints": 3, "hours": 12},
  "assigneeTotals": [{"assignee": "Ada Lovelace", "tasks": 1, "storyPoints": 3, "hours": 12}]
}
```

The text output also prints a `BY ASSIGNEE` summary after the visible-task
total, sorted by story points descending. Tasks with no assignee are grouped as
`Unassigned`. If a task has multiple assignees, its full effort is counted for
each person so the summary reflects each person's potential load; therefore,
per-assignee totals may exceed the project total.

### `panda browse [list filters]`

Opens an interactive selector for local tasks and a read-only action menu for
the selected task. The task selector supports typing to narrow matches. Actions
include full task view, description, notes, attached files, verbose pending diff,
and a verified task link when available. Choose Back to return to the task
selector or Quit to exit.

Browse accepts the status, content, assignee, and reviewer filters documented
for `panda list`; list's `--wide` and `--json` options are not supported. It
requires a terminal and never changes task files or synchronization state. Use
`panda list` for non-interactive or scripted output.

```sh
panda browse
panda browse --current
panda browse --assignee me --reviewer none
```

### `panda create [title] [task field options]`

Creates a task locally with the next available ID: `draft-1`, `draft-2`, and so
on. Interactive terminals offer
selectors for task fields, labels, package, and assignees. The optional
description editor uses `$EDITOR`.

Supplying task field options enables noninteractive mode: no prompts or editor
are launched, and a title is required either positionally or with `--title`.
Task options include `--type`, `--priority`, `--complexity`, `--status`, `--package`,
repeatable `--assignee`, repeatable `--label`/`--remove-label`, and
`--description-file`. Use `none` to clear optional fields; `--label none` must
be used alone to clear all labels. Release tasks cannot have a package.

`--description-file <path>` reads UTF-8 Markdown from a file, and
`--description-file -` reads from stdin. Supplied description content is kept
byte-for-byte, including trailing newlines; empty descriptions are rejected.

| Option | Values and behavior |
| --- | --- |
| `--title <text>` | Set the title |
| `--type <value>` | `feature`, `bug`, `other`, or `release` |
| `--priority <value>` | `hotfix`, `high`, `medium`, or `low` |
| `--complexity <value\|none>` | `xs`, `s`, `m`, `l`, `xl`, `xxl`; `none` clears |
| `--status <value>` | `new`, `toDo`, `inProgress`, `inReview`, `completed`, `staging`, `released`, or `discarded` |
| `--package <name\|none>` | Set one local package or clear it |
| `--assignee <member>` | Repeat to set multiple synced members by name, email, or ID; names accept normalized partial matches; `none` clears all |
| `--label <name\|none>` / `--add-label <name>` | Add labels; `--label none` clears all |
| `--remove-label <name>` | Remove a label; repeat as needed |
| `--description-file <path\|->` | Read UTF-8 Markdown from a file or stdin (`-`) |

### `panda edit <taskNumber|draft-N> [task field options]`

Interactively edits a task's title, type, priority, complexity, and status, or
applies only fields explicitly supplied by options. Option-driven edits never
prompt and preserve omitted fields.

Other edit selectors:

- `panda edit <task> labels [name ...|none]` opens the label picker or applies
  names; `--remove=<name>` removes a label.
- `panda edit <task> package [name|none]` selects or clears a package.
- `panda edit <task> assignees [member ...|none]` selects synced eligible
  members by name, email, or ID; names accept normalized partial matches.
- `panda edit <task> json` opens `metadata.json` in `$EDITOR`.
- Press `e` when prompted to open `description.md` in `$EDITOR`.

Release tasks cannot be assigned to packages. Eligible assignee roles are
`teamLead`, `developer`, and `qa`.

Status values are `new`, `toDo`, `inProgress`, `inReview`, `completed`,
`staging`, `released`, and `discarded`. On synced tasks, selecting `discarded`
stages the same dedicated discard action as `panda discard`; selecting another
status cancels a pending discard. On drafts, it sets the status for creation.

### `panda assign <taskNumber|draft-N> <person[,person...]|none>`

Assigns one or more eligible synced project members to a task by name, email, or
ID. `none` clears all assignees. Names use normalized partial matching: for
example, `juan` can match `Juanma Espinosa`, and accents and punctuation are
ignored. Exact normalized names/emails/IDs take precedence over partial matches.
If a partial name matches multiple people, Panda reports the candidates; use a
full name, email, or ID to disambiguate. Eligible roles remain `teamLead`,
`developer`, and `qa`. Run `panda pull` first to sync current members.

### `panda assign --status <status,...> [--priority <priority,...>] [--type <type,...>]`

Interactively walks matching tasks in workflow status and rank order. `--status`
is required; `--priority` and `--type` can further narrow the task set. Values
can be comma-separated or supplied by repeating the option. Valid priorities are
`hotfix`, `high`, `medium`, and `low`; valid types are `feature`, `bug`, `other`,
and `release`. Release tasks are skipped unless `--type release` is explicitly
selected. Known statuses are `new`, `toDo`, `inProgress`, `inReview`, `completed`,
`staging`, `released`, and `discarded`.

For each task, Panda shows its current assignees and estimated story points/hours.
Press Enter to open a multi-select prefilled with the current assignees; Enter
saves the selected members as the complete assignee list, and `None (clear all)`
removes everyone. Press Esc to return to the task actions, `d` to view the
description, `w` to show the per-assignee effort summary for all matching tasks
using currently saved assignments, `s` to skip, or `q` to stop. The filtered mode
requires a terminal and synced project members. Changes remain local until
`panda push`.

At the end, Panda prints an effort-load summary for all matching tasks, sorted
by story points descending. As in `panda list`, shared tasks count their full
effort for every assignee, and unassigned tasks appear under `Unassigned`.

```sh
panda assign 12345 juan
panda assign 12345 "Ada Lovelace,Grace Hopper"
panda assign 12345 none
panda assign --status toDo
panda assign --status toDo --priority high --type feature,bug
panda assign --status released --type release
panda push
```

### `panda view <taskNumber|draft-N> [--no-description] [--no-notes] [--json]`

Displays task metadata in an aligned table, followed by the local Markdown
description, attached file references, and notes. The metadata includes creator,
creation time, verified task link, and local sync status when available. The
active project is implicit in the selected workspace and is not repeated in the
metadata table.
Empty relationship fields display as `None`; staged notes and file uploads are
marked. Long terminal output uses `$PAGER` (default `less -FRX`) and remains
visible after leaving the pager; piped output uses the same table without color.

Options:

- `--no-description`: omit the Markdown description section.
- `--no-notes`: omit the notes section.
- `--json`: emit structured JSON with task metadata, Markdown description, notes,
  and sync context. JSON output is not paged. The suppression flags also omit
  their corresponding JSON fields.

```sh
panda view 12345
panda view 12345 --no-notes
panda view 12345 --json
```

### `panda link <taskNumber|draft-N>`

Prints a Markdown link for a synced task with a verified remote ID and baseline.
Links use the shared Solutions task view, ID `680b8e41805d3740e97cfecb`.
Locally created or unverifiable tasks cannot be linked.

### `panda files <taskNumber|draft-N>`

Lists attached file references stored in the task's local metadata and any
uploads staged for that task. File binaries are not stored in the workspace.

```sh
panda files 12345
```

#### `panda files upload <taskNumber|draft-N> <path> [--description <text>]`

Stages an upload by saving the source file's absolute path in sync state. The
file is uploaded and attached when `panda push` runs; keep it at that path until
then. After a successful push, Panda stores only the remote file reference in
task metadata. Draft tasks can also stage uploads.

```sh
panda files upload 12345 screenshot.png --description "Login screen"
panda files upload draft-1 report.pdf -d "Test report"
panda diff
panda push
```

#### `panda files download <taskNumber|draft-N> <file-id|index|name> [--output <path>]`

Downloads an attached file on demand using its file ID. Select a file by its
ID, its 1-based position from `panda files`, or an unambiguous name. By default,
Panda writes to an OS temporary file and prints its path. Use `--output` (or
`-o`) to choose a destination. Existing destination files are protected unless
`--force` (`-f`) is supplied. Requires a successful login.

```sh
panda files 12345
panda files download 12345 1
panda files download 12345 file-id --output ./screenshot.png --force
```

## Notes and status

### `panda add-note <taskNumber|draft-N> [text] [--file <path>]`

Stages a Markdown note for the next push. Use `--file <path>` to stage long-form
Markdown without putting it in command arguments; `--file -` reads from stdin.
`--file` cannot be combined with positional text. If neither text nor `--file`
is supplied, Panda opens a temporary note in `$EDITOR`. Empty notes are rejected.

```sh
panda add-note 12345 --file ./note.md
panda add-note 12345 --file - < ./note.md
panda diff --verbose
panda push
```

### `panda edit-note <taskNumber|draft-N> <note-file> [--file <path>]`

Edits one of the current user's synced notes. By default, the note opens in
`$EDITOR`. Use the filename shown by `panda view`; the `.md` extension is
optional. Use `--file <path>` to replace the note with Markdown from a file, or
`--file -` to read from stdin. Review changes with `panda diff --verbose` and
publish with `panda push`.

```sh
panda view 12345 --json
panda edit-note 12345 20261028T120000.000Z-username --file ./revised-note.md
panda diff --verbose
panda push
```

### `panda discard <taskNumber|draft-N> [note]`

Stages the task status as `discarded`. An optional Markdown note is staged with
the status change. `panda push` applies it.

### `panda clean [taskNumber|draft-N] [--yes|-y]`

Restores the last-synced task files and discards staged notes, uploads, and
other staged actions. It does not delete local upload source files. With no task
identifier, it also restores project catalogs and asks for confirmation;
`--yes` skips that confirmation. Unpushed locally created tasks are removed.
Local drafts can still be removed if their sync-state file is missing. A synced
task without a baseline cannot be restored; run `panda pull` to recreate its
baseline. Whole-project clean continues through tasks without baselines and
reports them after cleaning the recoverable tasks and restoring catalogs.

Existing `local-*` task IDs remain accepted for workspaces created by older
Panda versions.

## Project catalogs

`panda labels` and `panda packages` list the active project's local catalogs.
Each supports `create`, `edit`, and `delete`; changes remain local until
`panda push`. `panda pull` merges remote catalog changes and reports conflicts.

Label descriptions are plain text. Package descriptions are Markdown locally
and convert to/from HTML during synchronization. Packages also support start
date and estimation fields.

```sh
panda labels create security "Security review"
panda packages create "Billing migration"
panda labels edit security
panda packages delete "Billing migration"
```
