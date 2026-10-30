# Development and releases

## Requirements

- Go 1.26.10 or newer (as specified in `go.mod`).

## Local development

Run the local check suite from the repository root:

```sh
make check
go test -race ./...
```

`make check` verifies formatting and runs `go vet ./...`, `go test ./...`, and
`go build ./...`. Format Go files with `make fmt`.

To install the working checkout for local use:

```sh
go install ./cmd/panda
```

The binary is installed into `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is
unset. Ensure that directory is on `PATH`.

## Continuous integration

`.github/workflows/ci.yml` runs on pushes and pull requests across Linux, macOS,
and Windows. It checks formatting, vet, tests, and builds; Linux also runs the
race detector, static analysis, and vulnerability scanning. CI validates changes
but does not publish binaries or create releases.

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
