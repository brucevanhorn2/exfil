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
- ✅ Remote pane stays empty (with a "press `s` to connect" hint) until a real SSH connection is made — never defaults to browsing the local filesystem in normal use; `-t` opts back into that for local-to-local test transfers
- ✅ Gradient/neon chrome (`internal/ui/gradient.go`): borders, titles, and the progress bar render as a primary→secondary gradient instead of a flat color; unfocused panes use a muted (50%-toward-black) variant
- ✅ CI (GitHub Actions): build, `go vet`, `gofmt` check on every push
- ✅ File operations: delete (`d`, with Y/N confirm — works on the cursor file or all marked files), rename (`r`), mkdir (`m`), on both local and remote (SFTP) panes; all three refresh the current pane's listing afterward. Delete is intentionally non-recursive — `os.Remove`/`sftp.Client.Remove` both reject non-empty directories, which is what keeps this in scope without extra logic (recursive delete tracked separately in issue #15)

**What's genuinely left** (not urgent, not blocking normal use):
- Directory copy (currently shows "not supported")
- Recursive directory delete (issue #15) and view/edit operations
- Multi-host sessions (one SSH connection at a time)
- Transfer cancellation (Ctrl+C kills the whole app, partial files left on disk)
- UI logic (host form validation, path navigation) is undertested relative to `internal/fsys`/`internal/ui` file-op coverage

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

Outside `-t` test mode, the remote pane's `FS`/`Cwd` are never read before a real SSH connection: `Model.Init()` skips its `Refresh()`, `View()` shows `BrowserPane.EmptyMessage` instead of a listing, and `enqueueCopyDirection` refuses any transfer touching the remote pane while `!m.connected`. This prevents the remote pane from ever being mistaken for a live host. With `-t` (`Model.testMode`), it behaves as a `LocalFS` rooted at `/` from startup, which is what makes local-to-local testing possible without a live host — see README's "Testing locally" section.

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

### Gradient chrome (`internal/ui/gradient.go`)

`gradientBox`/`gradientText` replace lipgloss's single-flat-color border/title styles everywhere a pane is bordered — a `lipgloss.Style` only holds one color, not enough to interpolate a gradient, so `Theme` also stores raw `PrimaryColor`/`SecondaryColor`/`MutedPrimaryColor`/`MutedSecondaryColor` (`lipgloss.Color`) values alongside its derived styles. The gradient runs diagonally (top-left to bottom-right) by character position; focused panes use the vivid primary/secondary pair, unfocused panes use the muted (50%-toward-black) pair. `gradientBox`'s `width`/`height` match `lipgloss.Style`'s own `Width()`/`Height()` convention (interior size, not counting border columns/rows) — width wraps overflowing content (via lipgloss's own reflow), height is a floor that pads shorter content but never truncates taller content. The About screen's ASCII logo keeps its own independent fixed cyan→purple gradient (`gradientLogo`, `logoFrom`/`logoTo`), unrelated to the user's theme colors.

### Versioning

`internal/version.Version` defaults to `"dev"`; `make build` overrides it via `-ldflags` with `git describe --tags --always --dirty`. Shown on the About screen (`?`). No git tags exist yet — tag `v0.1.0` when ready to cut a first real release.

## Known Limitations

- Directories can't be copied (shows "not supported")
- Delete is non-recursive (empty dirs/files only — see issue #15); no view/edit
- No 1Password integration (explicitly deferred by user)
- Transfer cancellation not implemented
- Only one SSH connection per session

## Testing

```bash
make build
./exfil
```

To test transfers without SSH, pass `-t` (remote pane then defaults to local filesystem at `/` instead of showing the disconnected placeholder):
```bash
mkdir -p /tmp/exfil-test/{a,b}
echo hi > /tmp/exfil-test/a/file.txt
./exfil -t
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
