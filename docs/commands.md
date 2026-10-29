# Command reference

Run `panda --help` for the command index or `panda help <command>` for a
command's manual page. The same manual is available with
`panda <command> --help`.

## Setup and configuration

### `panda login [email]`

Authenticates with Solutions and saves the session token in Panda's private
user configuration. If the email is omitted, Panda prompts for it. The password
is prompted for and hidden when entered in a terminal.

### `panda config set project <name>`

Looks up a Solutions project by name and selects it as the active project.
Requires a successful login.

### `panda config get project`

Prints the active project name and ID.

### `panda path`

Prints the active project's workspace path without creating it.

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

### `panda push`

Checks for remote changes before sending local task and catalog edits. It skips
unchanged tasks, applies staged notes and discards, and advances synchronization
baselines after successful API operations. Overlapping edits are reported
instead of overwritten.

### `panda diff [taskNumber|localId] [--verbose|-v] [--summary|-s]`

Shows pending task and project-catalog changes. An optional task identifier
limits output to one task.

- Default output summarizes changed fields and descriptions.
- `--verbose` shows changed Markdown description and note lines with `-` and `+`
  prefixes.
- `--summary` prints counts of edited, discarded, and new tasks.
- Long terminal output uses `$PAGER`, defaulting to `less -FRX`; leaving the
  pager keeps its output visible. Piped output is printed directly.

## Tasks

### `panda list [status filters] [other filters]`

Lists local tasks grouped in workflow order and sorted by their fetched rank.
The default statuses are `new`, `toDo`, `inProgress`, `inReview`, and
`completed`; `staging`, `released`, and `discarded` are hidden by default.
Run `panda pull` to refresh local task data and rank values.

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

Complexity totals map `xs` to 1 SP/4 hours, `s` to 2/8, `m` to 3/12, `l` to
5/20, `xl` to 8/32, and `xxl` to 13/52. Unknown values contribute zero and
display dashes.

```sh
panda list --current
panda list --staging
panda list --status 'new,toDo' --no-complexity
panda list --no-description --no-package --no-labels
```

### `panda create [title] [task field options]`

Creates a task locally with a temporary ID. Interactive terminals offer
selectors for task fields, labels, package, and assignees. The optional
description editor uses `$EDITOR`.

Supplying task field options enables noninteractive mode: no prompts or editor
are launched, and a title is required either positionally or with `--title`.
Task options include `--type`, `--priority`, `--complexity`, `--package`,
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
| `--package <name\|none>` | Set one local package or clear it |
| `--assignee <member>` | Repeat to set multiple synced members; `none` clears all |
| `--label <name\|none>` / `--add-label <name>` | Add labels; `--label none` clears all |
| `--remove-label <name>` | Remove a label; repeat as needed |
| `--description-file <path\|->` | Read UTF-8 Markdown from a file or stdin (`-`) |

### `panda edit <taskNumber|localId> [task field options]`

Interactively edits a task, or applies only fields explicitly supplied by
options. Option-driven edits never prompt and preserve omitted fields.

Other edit selectors:

- `panda edit <task> labels [name ...|none]` opens the label picker or applies
  names; `--remove=<name>` removes a label.
- `panda edit <task> package [name|none]` selects or clears a package.
- `panda edit <task> assignees [member ...|none]` selects synced eligible
  members by name, email, or ID.
- `panda edit <task> json` opens `metadata.json` in `$EDITOR`.
- Press `e` when prompted to open `description.md` in `$EDITOR`.

Release tasks cannot be assigned to packages. Eligible assignee roles are
`teamLead`, `developer`, and `qa`.

### `panda assign <taskNumber|localId> <role[,role...]|none>`

Assigns every synced project member matching one or more eligible roles. Roles
can be comma- or space-separated. `none` clears all assignees. Run `panda pull`
first to sync current members.

### `panda view <taskNumber|localId>`

Displays task metadata, the local Markdown description, and notes. Staged notes
are marked. Long terminal output uses `$PAGER` (default `less -FRX`) and remains
visible after leaving the pager; piped output is printed directly.

### `panda link <taskNumber|localId>`

Prints a Markdown link for a synced task with a verified remote ID and baseline.
Links use the shared Solutions task view, ID `680b8e41805d3740e97cfecb`.
Locally created or unverifiable tasks cannot be linked.

## Notes and status

### `panda add-note <taskNumber|localId> [text]`

Stages a Markdown note for the next push. If text is omitted, Panda opens a
temporary note in `$EDITOR`. Empty notes are rejected.

### `panda edit-note <taskNumber|localId> <note-file>`

Opens one of the current user's synced notes in `$EDITOR`. Use the filename
shown by `panda view`; the `.md` extension is optional. Review changes with
`panda diff --verbose` and publish with `panda push`.

### `panda discard <taskNumber|localId> [note]`

Stages the task status as `discarded`. An optional Markdown note is staged with
the status change. `panda push` applies it.

### `panda clean [taskNumber|localId] [--yes|-y]`

Restores the last-synced task files and discards staged actions. With no task
identifier, it also restores project catalogs and asks for confirmation;
`--yes` skips that confirmation. Unpushed locally created tasks are removed.

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
