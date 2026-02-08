## Purpose
You are working in a Go (Golang) codebase. Make changes that are consistent with existing
structure, style, and tooling. Prefer small, correct, well-tested increments.

## First steps (always)
- Scan the repository layout before editing.
- Identify the Go module path from `go.mod`.
- Check for existing conventions in similar packages/files before introducing new patterns.

**Update documentation**:
- After a feature is implemented, **update `README.md`** to reflect the new behavior, usage, commands, configuration, or examples.

## Go standards & style
- Follow `gofmt` formatting and standard Go idioms.
- Prefer clarity over cleverness.
- Avoid unnecessary abstractions.
- Keep exported APIs documented with Go doc comments.
- Handle errors explicitly; wrap errors where helpful with context.

## Commands (use these unless repo specifies otherwise)
- Format: `gofmt -w .`
- Tests: `go test ./...`
- Vet: `go vet ./...`
- Tidy deps: `go mod tidy`

If the repo provides a `Makefile`, `Taskfile.yml`, or CI scripts, prefer those commands.

## Sandbox note
- In restricted sandbox environments, tests that open local TCP listeners (notably in `network/`) may fail with `socket: operation not permitted`.
- If that happens, rerun the relevant test command with elevated permissions instead of treating it as an application failure.

## Editing rules
- Don’t commit or log secrets.
- Don’t change public APIs without checking for downstream usage.
- Don’t introduce new dependencies unless necessary; justify them briefly in the PR/notes.
- Prefer updating existing files over creating new ones unless there’s a clear benefit.

## Definition of done
A change is considered complete when:
- Code compiles and tests pass (`go test ./...`).
- Code is formatted (`gofmt`).
- Any new behavior is covered by tests or clear manual verification steps.
- **`README.md` is updated** for any newly implemented feature.
