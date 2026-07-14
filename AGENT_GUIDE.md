# Agent Handoff Guide — exfil TUI SCP Client

**For AI agents picking up this project:** This guide explains the project state, architecture, and how to contribute without needing Bruce's original context.

## TL;DR — Project Status

**What it is:** A cyberpunk terminal UI SCP/SFTP client in Go. User wanted to stop using `scp` and bloated GUI clients to move files between machines (wintermute ↔ laptop).

**Current state:** MVP is functionally complete and verified end-to-end against a real remote host (SSH connect, host add/edit, directional transfers, live progress).

**What works now:**
```bash
make build
./exfil
# 's' to open the Site Manager, pick a saved host, Enter to connect
# Navigate panes with Tab/↑/↓/Enter/Backspace, mark files with Space
# '→' pushes selected file(s) local→remote, '←' pulls remote→local
# '?' opens an About screen (logo, version, license)
```

**What's left:** Directory copy, multi-host sessions, view/edit operations, transfer cancellation, and broader UI test coverage. None of these block normal single-host file transfer use.

**GitHub issues:** See https://github.com/brucevanhorn2/exfil/issues — treat as historical/roadmap notes; the SSH-wiring issues referenced there are done.

---

## Architecture (The Hard Bits Explained)

### Concurrency Model — Why It's Safe

This is the trickiest part. exfil uses **CSP discipline** (Communicating Sequential Processes):

```
Worker goroutines (3x)          UI goroutine (single-threaded)
    |                                  |
    +-- job from jobsCh          <- jobsCh (buffered, cap 256)
    |
    +-- do file transfer
    |
    +-- send progress msg --> eventsCh (buffered, cap 64)
                                  ^
                                  |
                            Model.Update() receives msg
                            Mutates transfer state safely
                            Returns waitForEvent(eventsCh) to re-arm subscription
```

**Key rule:** Worker goroutines **NEVER touch the Model directly**. They only send messages. All state mutations happen in `Update()`, which is single-threaded.

**Why this matters:** No mutexes needed, no race conditions, safe to have 3 workers running concurrently. Bubbletea's Elm architecture (pure functions + message dispatch) makes this possible.

**Where to find it:**
- `internal/transfer/queue.go` — spawns 3 workers pulling from `jobsCh`
- `internal/transfer/copy.go` — worker writes progress to `eventsCh` via `io.TeeReader` wrapper
- `cmd/exfil/main.go` — creates channels, starts workers, passes `eventsCh` to Model
- `internal/ui/app.go` — Model.Update handles transfer messages, re-arms `waitForEvent()`

### FileSystem Abstraction — Avoiding Duplication

Both panes (local and remote) use the same `fsys.FileSystem` interface:

```go
type FileSystem interface {
    ReadDir(path string) ([]Entry, error)       // List directory
    Join(elem ...string) string                 // Join paths
    Home() (string, error)                      // User home
    Open(path string) (io.ReadCloser, error)   // Read file
    Create(path string) (io.WriteCloser, error) // Write file
    Stat(path string) (*Entry, error)          // File info
    Remove(path string) error                   // Delete file or empty dir
    RemoveAll(path string) error                 // Recursive delete
    Rename(oldPath, newPath string) error        // Rename/move
    Mkdir(path string) error                     // Create directory
}
```

**Implementations:**
- `LocalFS` — wraps `os.ReadDir`, `filepath.Join`, `os.Open`/`os.Create`
- `RemoteFS` — wraps `sftp.Client.ReadDir`, `path.Join`, `client.Open`/`client.Create` (POSIX paths)

**Why this matters:** The browser pane code (`internal/ui/browser.go`) doesn't know or care which filesystem it's browsing.

**How SSH wiring actually works** (`internal/ui/app.go`):
- `Model.Init()` refreshes both panes at startup — local pane with `LocalFS`, remote pane also with `LocalFS` rooted at `/` (lets you test transfers locally, see README)
- Pressing `s` opens the Site Manager (`HostPickerPane`); `Enter` on a host triggers `connectSSH()`, a `tea.Cmd` that dials off the UI thread and returns an `sshConnectedMsg`
- On success, `sshConnectedMsg`'s handler wraps the `*sftp.Client` in a `RemoteFS` and assigns it to `m.remotePane.FS`, then kicks off an async `readDirCmd` to list the configured `remote_path`
- From then on the remote pane behaves identically to local — same `BrowserPane`, same key handlers, same transfer engine

### Transfer Engine — Cross-Filesystem Copies

```go
func RunWithFS(job Job, events chan tea.Msg, src FileSystem, dst FileSystem)
```

`enqueueCopyDirection(srcPane, dstPane *BrowserPane)` in `app.go` builds a `transfer.Job` carrying each pane's `FS`, so the worker doesn't need to know whether it's a local copy, upload, or download — it just calls `Open` on `src` and `Create` on `dst`.

**Progress tracking:** `progressWriter` wraps the destination, throttles to ~6 msgs/sec, emits `TransferProgressMsg`.

---

## How to Build & Run

### Prerequisites
```bash
go 1.26+ (see go.mod)
SSH keys in ~/.ssh (id_ed25519, id_rsa, or id_ecdsa) — for remote connections
ssh-agent running (optional, but preferred)
```

### Build
```bash
cd /home/bruce/Projects/exfil
make build   # embeds version via git describe; plain `go build` leaves it as "dev"
```

### Run
```bash
./exfil
```

### Logs
```bash
tail -f /tmp/exfil.log
```

---

## Testing Without SSH

The remote pane defaults to a local filesystem at `/` until you connect, so local-to-local testing works out of the box:

```bash
mkdir -p /tmp/test/{src,dst}
echo "test file" > /tmp/test/src/testfile.txt

./exfil
# Navigate local (left) pane to /tmp/test/src
# Navigate remote (right) pane to /tmp/test/dst
# Space to select testfile.txt, '→' to push it across
# Watch the progress bar in the Transfer Queue pane
```

Verify integrity:
```bash
sha256sum /tmp/test/src/testfile.txt /tmp/test/dst/testfile.txt
```

---

## Code Structure

```
exfil/
├── cmd/exfil/main.go              # Entry point — creates channels, starts workers
├── internal/
│   ├── config/config.go           # Site manager (hosts.yaml Load/Save)
│   ├── sshclient/client.go        # SSH dial (agent auth + key fallback)
│   ├── version/version.go         # Build-time version string (set via -ldflags)
│   ├── fsys/
│   │   ├── fsys.go                # FileSystem interface
│   │   ├── local.go               # LocalFS implementation
│   │   └── remote.go              # RemoteFS implementation (wraps sftp.Client)
│   ├── transfer/
│   │   ├── types.go               # Job struct, Direction enum
│   │   ├── queue.go                # StartWorkers() — spawns 3 goroutines
│   │   ├── copy.go                # Run/RunWithFS — transfer engine + progress
│   │   └── copy_smoke_test.go     # Only test file in the repo so far
│   └── ui/
│       ├── app.go                 # Model (Bubbletea), Update/View, screen routing
│       ├── browser.go             # BrowserPane — reused for local & remote
│       ├── queuepane.go           # QueuePane — transfer queue display (height-capped)
│       ├── hostpicker.go          # HostPickerPane — Site Manager screen
│       ├── hostform.go            # HostFormPane — add/edit host form
│       ├── about.go               # AboutPane — logo/version/license screen
│       └── theme.go               # Cyberpunk color scheme
├── .github/workflows/ci.yml       # build + go vet + gofmt check
├── Makefile                       # `make build` injects version via ldflags
├── LICENSE                        # MIT
└── README.md                      # User-facing docs
```

**The critical files for understanding:**
1. `cmd/exfil/main.go` — Concurrency setup
2. `internal/ui/app.go` — State machine & message handling (the biggest file, start here)
3. `internal/transfer/copy.go` — Transfer logic
4. `internal/fsys/fsys.go` — Why the abstraction matters

---

## What's Actually Left (Pick Up Here)

None of these block normal use; pick whichever matches what you're asked to do:

1. **Directory copy support** — currently `enqueueCopyDirection` in `app.go` skips directories with a "not supported" status message.
2. **View/edit operations** — delete (`d`, including recursive delete of non-empty directories), rename (`r`), and mkdir (`m`) are implemented (`internal/ui/fileops.go`, `internal/fsys`); viewing/editing a file's contents in-app is not.
3. **Multi-host sessions** — `Model` holds a single `*ssh.Client`/`*sftp.Client`; switching hosts mid-session isn't supported.
4. **Test coverage** — `internal/fsys` and `internal/ui` now have real coverage (see `local_test.go`, `app_test.go`), but `HostFormPane.buildHost()` validation, `BrowserPane.Back()` path logic, and `BrowserPane.ensureVisible()` scrolling are still good next targets.
5. **Transfer cancellation** — Ctrl+C kills the whole app; partial files are left on disk.

---

## Common Gotchas & Debugging

### Transfer appears queued but never runs

Check:
- `jobsCh` is passed to Model (see `main.go`)
- Workers are started before `tea.NewProgram()` (see `main.go`)
- `enqueueCopyDirection()` sends to `m.jobsCh` (see `app.go`)

### UI freezes during SSH connect

The SSH dial runs as a `tea.Cmd` in its own goroutine, so the UI should stay responsive. If it freezes, check `sshclient.Dial()` for a blocking passphrase prompt or slow DNS/network, and check `/tmp/exfil.log`.

### Remote pane shows nothing after "connecting"

Likely causes:
- SSH auth failed silently → check logs
- `remote_path` in `hosts.yaml` doesn't exist on the remote → `readDirCmd` will report the error in `m.statusMsg`

### Editing a host doesn't save the right one

Shouldn't happen — `HostFormPane` keys edits off the host's original `Name`, not list position, specifically to avoid this. If you see it, check `HostFormPane.Save()` in `hostform.go`.

---

## Key Design Decisions (Why It's Built This Way)

| Decision | Rationale |
|----------|-----------|
| **Bubbletea + Lipgloss** | Elm architecture is perfect for TUI state machines. Lipgloss handles styling. |
| **CSP concurrency (no mutexes)** | Simpler, safer, easier to reason about. Workers & UI never fight over state. |
| **FileSystem interface** | Single pane code works for local or remote. Swaps at connect time, not compile time. |
| **3 concurrent workers** | Good balance: enough parallelism, not too many goroutines. Bounded by channel recv. |
| **SSH-agent first, keys second** | Matches user's existing SSH setup. No extra secrets to manage. |
| **YAML config (not JSON)** | Supports comments. Humans can edit `~/.config/exfil/hosts.yaml` by hand. |
| **No password prompts** | Set up keys once, use many times. Agent or key files only. |
| **Edit-by-name, not by-index** | A positional index into the host list can go stale if the file changes between load and save; name lookup can't silently corrupt the wrong entry. |
| **Fixed-direction arrow transfers** | `→`/`←` always mean local→remote/remote→local regardless of pane focus — more predictable than focus-dependent direction. |

---

## If You Get Stuck

1. **Check CLAUDE.md** — implementation notes, patterns, current known limitations
2. **Read the code** — it's well-structured and intentional; comments explain the "why," not the "what"
3. **Check logs** — `/tmp/exfil.log` has detailed errors from workers
4. **Test locally first** — verify local-to-local transfer works before troubleshooting SSH
