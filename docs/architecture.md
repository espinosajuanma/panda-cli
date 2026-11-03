# Architecture

Panda is a local-first CLI. Commands read and update a project workspace; `pull`
and `push` reconcile that workspace with the Solutions API.

## Package responsibilities

```text
cmd/panda
  └── internal/app       command dispatch and command workflows
        ├── internal/ui  terminal styling, paging, prompts, selectors, and editor launch
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
  changes and staged actions. Staged file uploads retain a source-path reference
  until upload; their remote ID is saved until attachment succeeds. Binaries are
  never stored in the workspace.
- Mutating commands hold an exclusive workspace lock. HTTP GET requests use
  bounded retries for transient failures and honor cancellation.
- Merge and conflict rules should be pure computations where practical; file
  writes and API calls belong in orchestration/repository code.
- Baselines advance only for successfully synchronized data, preserving the
  existing conflict-recovery behavior.

## Read-only tools and machine output

- `list` and `diff` inspect the selected local workspace; `browse` provides an
  interactive read-only view over the same task/filter report. `browse` must not
  acquire the mutation lock or initialize/migrate the workspace.
- `doctor` is local-only and read-only. Workspace inspection helpers such as
  state-key enumeration and lock probing must not create files or change saved
  state.
- `list --json`, `diff --json`, `push --json`, and `doctor --json` emit
  versioned documents with `schemaVersion: 1`. Keep human-readable output and
  JSON rendering on the same report data so filters, ordering, and counts agree.
- These reports do not change the workspace data format described above.

## Change discipline

When moving code, preserve command text, API payload shapes, workspace data, and
exit behavior. Make structural moves separately from behavioral changes and run
`make check` after each refactor step.
