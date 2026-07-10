# exfil — cyberpunk TUI SCP/SFTP client

A terminal-based file transfer client for Linux (Ubuntu/Pop_OS) built with Go, Bubbletea, and Lipgloss.

**📖 Documentation:**
- **User guide** — This README
- **[Developer/Agent guide](AGENT_GUIDE.md)** — Architecture, concurrency model, how to contribute
- **[Implementation notes](CLAUDE.md)** — Detailed status, code patterns, critical path
- **[GitHub issues](https://github.com/brucevanhorn2/exfil/issues)** — Roadmap and task tracking

## MVP Status

**Current (M1-M4 partial):** Local file browsing and copying with a live transfer queue and progress bars.

- ✅ Dual-pane file browser (left=local, right=local for MVP testing)
- ✅ Transfer queue pane with progress bars
- ✅ File navigation (up/down/enter/backspace)
- ✅ File selection (space to toggle)
- ✅ Copy operations (local-to-local for testing; ready for SFTP)
- ✅ Concurrent transfer worker pool (3 workers, bounded concurrency)
- ✅ Live progress reporting with speed calculation
- ✅ Cyberpunk theming (dark, magenta/cyan/green accents)

**Not yet complete (M3, M4 final):**

- ⏳ SSH/SFTP connection integration
- ⏳ Site manager (saved host profiles)
- ⏳ Remote pane wiring to SFTP filesystem
- ⏳ Full theming pass and footer key-hints

## Building

```bash
cd /home/bruce/Projects/exfil
go build -o exfil ./cmd/exfil
```

## Running

```bash
./exfil
```

Controls:
- **Tab** — switch focus between panes
- **↑/↓** — navigate within pane
- **Enter** — enter directory
- **Backspace** — go back to parent directory
- **Space** — toggle select file
- **c** — copy selected files to other pane
- **q** — quit

## Architecture

### Concurrency model

Two patterns:

1. **One-shot async work** (directory listing): Plain `tea.Cmd` run in a goroutine by Bubbletea, result delivered as a message.
2. **Continuous streams** (transfer progress): Channel + re-arming `tea.Cmd` ("subscription" pattern):
   - `eventsCh` collects progress from worker goroutines
   - N=3 worker goroutines pull jobs from `jobsCh` and stream progress back
   - `Update` loop receives messages single-threaded, updates transfer state
   - No mutexes — strict CSP discipline ("share nothing, communicate everything")

### Package layout

- `cmd/exfil/main.go` — Entry point, starts worker pool
- `internal/config/` — Site manager persistence (hosts.yaml)
- `internal/sshclient/` — SSH/SFTP dial (agent auth + identity fallback)
- `internal/fsys/` — Filesystem abstraction (LocalFS, RemoteFS)
- `internal/transfer/` — Copy engine, worker pool, progress tracking
- `internal/ui/` — Bubbletea TUI (panes, theme, app model)

### File system abstraction

`fsys.FileSystem` interface allows both panes to reuse the same navigation and rendering code:
```go
type FileSystem interface {
    ReadDir(path string) ([]Entry, error)
    Join(elem ...string) string
    Home() (string, error)
    Open(path string) (io.ReadCloser, error)
    Create(path string) (io.WriteCloser, error)
    Stat(path string) (*Entry, error)
}
```

`LocalFS` uses `os.ReadDir`, `filepath.Join`, etc. `RemoteFS` wraps an `*sftp.Client` using SFTP protocol calls.

## Next steps for completing the MVP

### 1. Wire up SSH/SFTP (M3)

In `internal/ui/app.go`, add:
- Host picker screen (already stubbed in `hostpicker.go`)
- SSH connection flow triggered on "connect" from picker
- `sshConnectedMsg` handler that wires up `RemoteFS` to the right pane
- Switch panes to use appropriate `FileSystem` (local vs. remote)

The `sshclient.Dial` function already supports:
- ssh-agent for key auth
- Fallback identity files (`~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`)
- No password/passphrase prompts

### 2. Site manager (M3)

`config.Load()` / `config.Save()` already work against `~/.config/exfil/hosts.yaml`. Need to add:
- Host picker navigation and display (UI part of M3)
- "Add host" form (minimal `bubbles/textinput` fields, save to YAML)
- "Connect ad hoc" flow for one-off hosts

### 3. Theming and polish (M5)

Already has baseline theme, but could add:
- Footer key-hints instead of inline status
- Rounded borders on focused pane only
- Dark background (ANSI color "0" instead of default)
- Animation/spinner during SSH connect

## Testing locally

To test the transfer queue without SSH, use both panes as local directories:

```bash
mkdir -p /tmp/exfil-test/{a,b}
echo "test content" > /tmp/exfil-test/a/file1.txt
# In the app, navigate both panes to /tmp/exfil-test
# Left pane: /a, Right pane: /b
# Select file1.txt in left, press 'c' to copy to right
```

## Known limitations (MVP scope)

- Directories cannot be copied (shows "not supported" message)
- Single SSH connection per session (no switching between hosts mid-run)
- No delete, rename, mkdir, or view/edit operations
- No 1Password integration
- No recursive directory sync

## Logs

Application logs go to `/tmp/exfil.log` for debugging.
