# Architecture

Panda is a local-first CLI. Commands read and update a project workspace; `pull`
and `push` reconcile that workspace with the Solutions API.

## Package responsibilities

```text
cmd/panda
  └── internal/app       command dispatch and command workflows
        ├── internal/ui  terminal styling, paging, prompts, and editor launch
        ├── internal/task task field/status/effort definitions
        ├── internal/catalog catalog kind and field definitions
        ├── internal/diff line diff algorithm
        ├── internal/workspace filesystem layout and state persistence
        ├── internal/solutions Solutions HTTP API client
        ├── internal/config user configuration and workspace root
        └── internal/content Markdown/HTML conversion
```

`internal/jsonutil` contains helpers for the JSON-shaped maps used at the
Solutions and metadata boundaries. Keep those maps at the boundary so unknown
API fields continue to survive local reads, edits, and writes. Stable
application concepts such as sync state and remote notes should use explicit
types.

Interactive selector rendering and terminal-mode handling are still
implemented in `internal/app`; moving them into `internal/ui` is a remaining
extraction step.

The intended dependency direction is from command entry points toward domain,
filesystem, and transport helpers. Lower-level packages must not import
`internal/app`.

## Workspace compatibility

The workspace layout under the OS user config directory is user data. Refactors
must preserve task directory names, `metadata.json` fields, Markdown files,
state-key encoding, catalog file names, and note filenames unless a separately
reviewed migration is introduced.

## Sync responsibilities

- Pull fetches project catalogs, members, and tasks; it merges remote changes
  against the saved baseline while retaining independent local changes.
- Push checks for overlapping remote edits before sending local task/catalog
  changes and staged actions. Staged file uploads retain only a source-path
  reference until push; successful sync stores remote file IDs, not binaries.
- Merge and conflict rules should be pure computations where practical; file
  writes and API calls belong in orchestration/repository code.
- Baselines advance only for successfully synchronized data, preserving the
  existing conflict-recovery behavior.

## Change discipline

When moving code, preserve command text, API payload shapes, workspace data, and
exit behavior. Make structural moves separately from behavioral changes and run
`make check` after each refactor step.
