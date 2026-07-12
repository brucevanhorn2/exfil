# exfil — Implementation Notes

## Project Goal

A cyberpunk-themed terminal UI SCP/SFTP client for Linux. Replace `scp` and bloated GUI clients with a fast, local, no-account TUI written in Go.

## Current Status

The MVP is functionally complete and has been verified end-to-end against a real remote host:

- ✅ Dual-pane file browser, local and remote (SFTP) — both panes share the same `fsys.FileSystem` interface
- ✅ SSH/SFTP connection via the Site Manager (`s` to open, `Enter` to connect)
- ✅ Add/edit hosts from within the app (`n`/`e`), saved to `~/.config/exfil/hosts.yaml`
- ✅ Directional transfers: `→` pushes local→remote, `←` pulls remote→local, regardless of which pane has focus
- ✅ Concurrent transfer worker pool (3 workers), live progress bars, speed calculation
- ✅ Transfer queue pane with a capped height (shows the most recent transfers; never grows the layout past the terminal)
- ✅ Cyberpunk theming; both browser panes force-fill their assigned width/height
- ✅ About screen (`?`) — ASCII logo, version (via `git describe`, injected by `make build`), license
- ✅ Selectable lingo packs (`internal/i18n`) and free-form hex theme colors, via a dedicated Settings screen (`S`)
- ✅ CI (GitHub Actions): build, `go vet`, `gofmt` check on every push

**What's genuinely left** (not urgent, not blocking normal use):
- Directory copy (currently shows "not supported")
- Delete / rename / mkdir / view-edit operations
- Multi-host sessions (one SSH connection at a time)
- Transfer cancellation (Ctrl+C kills the whole app, partial files left on disk)
- Only one test file exists (`internal/transfer/copy_smoke_test.go`); UI logic (host form validation, path navigation) is undertested

## Code Patterns & Guidelines

### Concurrency (CSP discipline)

- Worker goroutines **only send messages, never touch Model**
- Model.Update is single-threaded, mutates state safely
- eventsCh is buffered (cap 64)
- jobsCh is buffered (cap 256)
- On transfer message, always return `waitForEvent(m.eventsCh)` to re-arm the subscription

### FileSystem interface

Both panes implement the same `fsys.FileSystem` interface. This eliminates code duplication:
- `ReadDir(path)` → sorted entries
- `Join(elem...)` → path.Join (POSIX) or filepath.Join (local)
- `Open/Create` → io.ReadCloser / io.WriteCloser
- `Stat(path)` → single Entry

The remote pane defaults to a `LocalFS` rooted at `/` until an SSH connection is made (both panes get an initial `Refresh()` in `Model.Init()`), which is what makes local-to-local testing possible without a live host — see README's "Testing locally" section.

### Transfer progress

`ProgressWriter` wraps `io.Writer`, throttles to ~6 msgs/sec, emits `TransferProgressMsg`. This keeps `eventsCh` from flooding on fast copies.

`Model.transferDest` maps each in-flight transfer ID to which pane ("local"/"remote") it's copying into. `TransferDoneMsg` looks up the destination pane and re-lists whatever directory it currently shows, so a completed transfer appears without navigating away and back. The entry is removed on both `TransferDoneMsg` and `TransferErrorMsg`.

### Screen state machine (`internal/ui/app.go`)

`Model.screen` selects between `ScreenBrowsing`, `ScreenHostPicker`, `ScreenAddHost`, `ScreenAbout`, and `ScreenSettings`. Each screen has its own `handle*Key` function; `Update()` routes `tea.KeyMsg` to the right one based on `m.screen`. `View()` swaps in the corresponding pane's rendering when not on `ScreenBrowsing`.

The browsing screen keeps a **persistent hint bar** (key bindings) separate from `m.statusMsg` (transient messages like "Connected to..." or errors) — don't let transient status overwrite the hints; they're rendered as two separate lines in `View()`.

### Config (hosts.yaml)

Located at `~/.config/exfil/hosts.yaml`. Loaded via `config.Load()`, saved via `cfg.Save()`. YAML format supports comments (though `cfg.Save()`'s `yaml.Marshal` doesn't preserve existing comments on rewrite). Host edits in the UI are keyed by the host's `Name` (not list position), so a stale index can't silently overwrite the wrong entry if the file changes between opening the picker and saving.

### SSH auth

`sshclient.Dial` tries:
1. ssh-agent (via `SSH_AUTH_SOCK` environment variable)
2. Fallback identity files in `~/.ssh`: `id_ed25519`, `id_rsa`, `id_ecdsa` (in that order)

No password/passphrase prompts.

### Lingo packs (`internal/i18n`)

Every user-facing string goes through `loc.T("message_id", args...)` rather than being hardcoded. Four packs — `plain`, `secretsquirrel`, `keyboardcowboy`, `corposlut` — live as embedded YAML catalogs in `internal/i18n/locales/`. `Localizer.T` falls back to `plain` for any key missing from the active pack, then to the raw message ID if even `plain` doesn't have it. Panes don't store `Theme`/`Localizer` at construction — `View()` takes them as parameters — so a Settings-screen change re-themes the whole app immediately without reconstructing anything.

### Versioning

`internal/version.Version` defaults to `"dev"`; `make build` overrides it via `-ldflags` with `git describe --tags --always --dirty`. Shown on the About screen (`?`). No git tags exist yet — tag `v0.1.0` when ready to cut a first real release.

## Known Limitations

- Directories can't be copied (shows "not supported")
- No delete, rename, mkdir, view/edit
- No 1Password integration (explicitly deferred by user)
- Transfer cancellation not implemented
- Only one SSH connection per session

## Testing

```bash
make build
./exfil
```

To test transfers without SSH (remote pane defaults to local filesystem at `/`):
```bash
mkdir -p /tmp/exfil-test/{a,b}
echo hi > /tmp/exfil-test/a/file.txt
# Navigate local pane to /tmp/exfil-test/a, remote pane to /tmp/exfil-test/b
# Select file.txt, press '→' to copy it across
```

CI runs `go build`, `go vet`, and a `gofmt` check on every push to `master`.

## Performance Notes

- Transfer speed is IO-bound, not CPU-bound
- 3 concurrent workers is a good balance; can increase if needed
- Progress message throttle (~150ms) prevents UI refresh thrashing
- SFTP is single TCP connection multiplexed by request ID (safe for concurrent ops from multiple goroutines)

## Future Extensions (Post-MVP)

- Bookmarks for frequent remote dirs
- Bidirectional sync
- Multi-host session switching
- Background queue persistence
- Bash/zsh integration (export/import bookmarks)
- Mouse support for pane clicking
- Search/filter files
- Stat view (permissions, owner, timestamps)
