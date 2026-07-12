# Contributing

This is currently a single-maintainer project, but the workflow below is what
CI enforces, so it's worth following even for solo changes.

## Build

```bash
make build
```

Embeds a version string via `git describe`. Plain `go build -o exfil ./cmd/exfil` also works but leaves the version as `dev`.

## Before committing

CI runs these on every push; run them locally first so you're not waiting on a red build:

```bash
go build ./...
go vet ./...
gofmt -l .    # should print nothing; if it does, run `gofmt -w <files>`
go test ./...
```

## Commit style

- Focus commit messages on *why*, not *what* — the diff already shows what changed.
- Prefer a few logically-grouped commits over one giant one, and over many tiny ones.
- Don't commit the compiled `exfil` binary (see `.gitignore`).

## Code conventions

See `CLAUDE.md` for architecture notes and established patterns (concurrency model, FileSystem abstraction, screen state machine) — new code should follow those rather than introducing a parallel approach.
