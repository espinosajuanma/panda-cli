# Development and releases

## Requirements

- Go 1.26.10 or newer (as specified in `go.mod`).

## Local development

Run the local check suite from the repository root:

```sh
make check
go test -race ./...
make coverage
```

`make coverage` writes `coverage.out` with cross-package attribution via
`-coverpkg=./...`. `make check` verifies formatting, runs `go vet ./...`, checks package layering
with `make arch-check`, runs `go test ./...`, and runs `go build ./...`. Format
Go files with `make fmt`. The architecture check lives in `internal/archtest`
and can be run directly with `go test ./internal/archtest`.

For a local cross-platform compile smoke test, run:

```sh
GOOS=windows GOARCH=amd64 go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
```

Before shipping command or JSON-schema changes, also review `panda --help`, the
command-specific manuals, and `docs/commands.md`. Workspace layout and persisted
state changes require an explicit compatibility review.

To install the working checkout for local use:

```sh
go install ./cmd/panda
```

The binary is installed into `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is
unset. Ensure that directory is on `PATH`.

## Continuous integration

`.github/workflows/ci.yml` runs the fast Linux format, vet, test, and build checks
on pushes to `main`. Pull requests, `v*` tags, and manual dispatches also run the
macOS/Windows matrix, race detector, Staticcheck, and vulnerability scan.
Superseded runs for the same ref are cancelled. CI validates changes but does
not publish binaries or create releases.

## Install from GitHub

The module path is `github.com/espinosajuanma/panda-cli`. After a version tag has
been pushed, users can install the newest tagged version with:

```sh
go install github.com/espinosajuanma/panda-cli/cmd/panda@latest
```

The existing `v1.0.0` tag predates this module path. The first release tag made
after the module-path update is the first one installable through this URL.

To pin an exact version, replace `latest` with a tag such as `@v1.1.0`. Go
builds the CLI from the tagged source; a binary-release workflow is not
required. Verify the installed command with `command -v panda` and
`panda --help`.
