# Workspace and task data

Panda is local-first. It stores task files, catalogs, and synchronization
baselines under the OS user configuration directory; no database is used.

Print the active workspace path with `panda path`.

## Directory layout

```text
{user-config-dir}/panda-dashboard/
  config.json
  workspaces/
    {projectName}/
      .panda/
        labels.json
        packages.json
        members.json
        state/
      tasks/
        {taskNumber-or-draft-N}/
          description.md
          metadata.json
          notes/
            {timestamp}-{authorName}.md
```

Configuration and workspace directories use private permissions where
supported by the operating system. Mutating commands hold an exclusive lock in
`.panda/workspace.lock` to prevent concurrent state changes. `metadata.json` stores task fields and
relationships, including task file IDs and names; descriptions and notes are
Markdown files. File binaries are not stored in the workspace. Uploads are
staged as source-path references in sync state until `panda push`; downloads
are fetched on demand into a temporary file unless an output path is supplied.

The `.panda/state` directory stores remote baselines, staged actions, and remote
task IDs. A locally created task uses the next `draft-N` ID until a successful
push assigns its Solutions task number and renames its directory. Older task
directories at the project root are migrated into `tasks/` when the workspace
is next opened; unrelated project files are left in place.

Draft numbering starts at `draft-1` and advances past the highest existing
draft. If the highest-numbered drafts are removed or pushed and renamed to
Solutions task numbers, a later draft may reuse those numbers. New tasks use
`draft-N`; legacy `local-*` task IDs remain accepted.

## Tasks and synchronization

Solutions tasks use the `dev.tasks` entity. Panda stores editable metadata such
as title, type, priority, complexity, package, and labels, and tracks status,
assignees, reviewers, and notes.

`pull` compares fetched data with the last-synced baseline. Independent local and
remote changes are merged; overlapping edits are reported as conflicts. Pull
stores task file references but never downloads their content. `push` checks for
remote changes before applying local edits and uploads staged files only when
they can be attached to a task. Baselines advance after successful operations.
Catalog baselines are stored alongside local labels and packages.

Run `panda clean <task>` to restore a task to its last-synced state. `panda
clean --yes` restores the entire project, including catalogs.

## Canonical task links

`panda link <taskNumber|draft-N>` prints a verified Markdown link:

```text
[#<taskNumber>](https://solutions.slingrs.io/prod/runtime/#views/680b8e41805d3740e97cfecb/readOnly/<verifiedTaskId>)
```

The `views/` segment always uses the shared Solutions task view ID
`680b8e41805d3740e97cfecb`. The final ID comes from `metadata.json` and must
match Panda's last-synced remote baseline; unpushed or unverifiable tasks cannot
be linked.
