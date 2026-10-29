# Panda CLI

[![Go CI](https://github.com/espinosajuanma/panda-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/espinosajuanma/panda-cli/actions/workflows/ci.yml)

A local-first command-line client for syncing Solutions tasks into files you can
edit with your preferred tools.

## Features

- Sync tasks, labels, packages, and team members with a local workspace.
- Edit task metadata, descriptions, and notes as files.
- Review changes and conflicts before pushing updates to Solutions.
- Use interactive selectors, scriptable options, and shell completions.

## Requirements

- Go 1.26 or newer.
- A Solutions account and access to a project.

## Installation

Install the latest tagged release with Go:

```sh
go install github.com/espinosajuanma/panda-cli/cmd/panda@latest
```

This requires a release tag made after the repository's Go module path was
updated to `github.com/espinosajuanma/panda-cli`.

Ensure Go's install directory is on your `PATH`: `GOBIN` when set, or usually
`$(go env GOPATH)/bin` otherwise. Verify the installation with:

```sh
command -v panda
panda --help
```

To pin a specific release, replace `@latest` with a version such as `@v1.1.0`.

## Quick start

```sh
panda login
panda config set project "Project Name"
panda pull
panda list
```

Edit the local task files, review pending changes with `panda diff`, and publish
them with `panda push`.

## Commands

| Command | Description |
| --- | --- |
| `panda pull` / `panda push` | Synchronize local changes with Solutions |
| `panda list` | List local tasks by workflow status |
| `panda create` / `panda edit` | Create or edit tasks locally |
| `panda diff` | Review pending changes and conflicts |
| `panda view <task>` | Display task details, description, and notes |
| `panda labels` / `panda packages` | Manage project catalogs locally |
| `panda link <task>` | Print a verified Markdown link to a synced task |

Run `panda help <command>` for command-specific help. See the full
[command reference](docs/commands.md) for flags, examples, and interactive
behavior.

## Configuration and workspace

Configuration and task files live under the OS user configuration directory in
`panda-dashboard/`. Run `panda path` to print the active workspace path. See
[workspace documentation](docs/workspace.md) for the file layout, task data, and
sync behavior.

## Development

```sh
make check
go test -race ./...
```

See [development and release instructions](docs/development.md) and the
[architecture guide](docs/architecture.md). The Solutions API schema is
available in [`assets/solutions-openapi.json`](assets/solutions-openapi.json).
