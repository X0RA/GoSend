## Purpose
You are working in a Go (Golang) codebase. Make changes that are consistent with existing
structure, style, and tooling. Prefer small, correct, well-tested increments.

**If you get confused** about the direction of the project, planned changes, or how pieces fit together, **refer to `PHASE-CHANGES.md`**. It describes the enhancement roadmap and the intent behind each phase.

## First steps (always)
- Scan the repository layout before editing.
- Identify the Go module path from `go.mod`.
- Check for existing conventions in similar packages/files before introducing new patterns.

**Update documentation**:
- After a feature or phase item is implemented, **update `README.md`** to reflect the new behavior, usage, commands, configuration, or examples.
- **Update the checkboxes in `PHASE-CHANGES.md`**: mark the completed items as checked (`[x]`) so progress is visible.

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
- **Checkboxes in `PHASE-CHANGES.md`** are updated: any completed phase items are marked as done (`[x]`).
