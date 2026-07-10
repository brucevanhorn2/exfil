# Agent Handoff Guide — exfil TUI SCP Client

**For AI agents picking up this project:** This guide explains the project state, architecture, and how to contribute without needing Bruce's original context.

## TL;DR — Project Status

**What it is:** A cyberpunk terminal UI SCP/SFTP client in Go. User wanted to stop using `scp` and bloated GUI clients to move files between machines (wintermute ↔ laptop).

**Current state:** MVP works locally (dual-pane file browser, transfer queue with progress bars, concurrent file copy). **Ready for SSH integration** — all the hard parts done, just needs UI wiring.

**What works now:**
```bash
./exfil                    # Launches TUI
# Navigate left/right panes, mark files with space, press 'c' to copy
# Watch progress bars update in real time as 3 concurrent workers copy files
```

**What's incomplete:** Remote panes don't connect to SSH yet. Everything else is done.

**GitHub issues:** See https://github.com/brucevanhorn2/exfil/issues for the full roadmap. Issues #1–3 are the critical path to MVP completion.

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
    ReadDir(path string) ([]Entry, error)      // List directory
    Join(elem ...string) string                // Join paths
    Home() (string, error)                     // User home
    Open(path string) (io.ReadCloser, error)  // Read file
    Create(path string) (io.WriteCloser, error) // Write file
    Stat(path string) (*Entry, error)         // File info
}
```

**Implementations:**
- `LocalFS` — wraps `os.ReadDir`, `filepath.Join`, `os.Open`/`os.Create`
- `RemoteFS` — wraps `sftp.Client.ReadDir`, `path.Join`, `client.Open`/`client.Create` (POSIX paths)

**Why this matters:** The browser pane code (`internal/ui/browser.go`) doesn't know or care which filesystem it's browsing. This eliminates ~200 lines of duplicated UI code.

**For SSH integration:** Once M3 is done, the remote pane will just be instantiated with `RemoteFS` instead of `LocalFS`. No UI changes needed.

### Transfer Engine — Ready for Cross-Filesystem Copies

`transfer.Run()` does local-only copy. But it has a sibling:

```go
func RunWithFS(job Job, events chan tea.Msg, src FileSystem, dst FileSystem)
```

**How it's used:**
- MVP: `Run()` → calls `RunWithFS(job, events, LocalFS{}, LocalFS{})`
- SSH future: `RunWithFS(job, events, RemoteFS{sftp}, LocalFS{})` for download
- SSH future: `RunWithFS(job, events, LocalFS{}, RemoteFS{sftp})` for upload

**Progress tracking:** `progressWriter` wraps the destination, throttles to ~6 msgs/sec, emits `TransferProgressMsg`. This prevents message flooding on fast copies.

---

## How to Build & Run

### Prerequisites
```bash
go 1.23+ (or whatever's in go.mod)
SSH keys in ~/.ssh (id_ed25519, id_rsa, or id_ecdsa)
ssh-agent running (optional, but preferred)
```

### Build
```bash
cd /home/bruce/Projects/exfil
go build -o exfil ./cmd/exfil
```

### Run locally (no SSH)
```bash
./exfil
# Navigate with arrow keys, mark with space, copy with 'c'
# Both panes start as local filesystems for testing
```

### Logs
```bash
tail -f /tmp/exfil.log
```

---

## Testing Without SSH (Recommended Starting Point)

### Local copy test
```bash
mkdir -p /tmp/test/{src,dst}
echo "test file" > /tmp/test/src/testfile.txt

# Launch app
./exfil
# Navigate left pane to /tmp/test/src
# Navigate right pane to /tmp/test/dst
# Mark testfile.txt with space, press 'c'
# Watch progress bar in bottom pane
# File appears in dst/ once done
```

### Verify file integrity
```bash
sha256sum /tmp/test/src/testfile.txt /tmp/test/dst/testfile.txt
# Should match
```

---

## Code Structure (Where Everything Lives)

```
exfil/
├── cmd/exfil/main.go              # Entry point — creates channels, starts workers
├── internal/
│   ├── config/config.go           # Site manager (hosts.yaml Load/Save)
│   ├── sshclient/client.go        # SSH dial (agent auth + key fallback)
│   ├── fsys/
│   │   ├── fsys.go                # FileSystem interface
│   │   ├── local.go               # LocalFS implementation
│   │   └── remote.go              # RemoteFS implementation (wraps sftp.Client)
│   ├── transfer/
│   │   ├── types.go               # Job struct, Direction enum
│   │   ├── queue.go               # StartWorkers() — spawns 3 goroutines
│   │   └── copy.go                # Run/RunWithFS — transfer engine + progress
│   └── ui/
│       ├── app.go                 # Model (Bubbletea), Update/View, messages
│       ├── browser.go             # BrowserPane — reused for local & remote
│       ├── queuepane.go           # QueuePane — transfer queue display
│       ├── hostpicker.go          # HostPickerPane — saved hosts screen (not wired yet)
│       └── theme.go               # Cyberpunk color scheme
└── README.md                       # User-facing docs
```

**The critical files for understanding:**
1. `cmd/exfil/main.go` — Concurrency setup
2. `internal/ui/app.go` — State machine & message handling
3. `internal/transfer/copy.go` — Transfer logic
4. `internal/fsys/fsys.go` — Why the abstraction matters

---

## How to Pick Up Work (GitHub Issues)

### Issues #1–3 (MVP Critical Path)

**[#1] SSH/SFTP connection wiring:**
- Add `connectSSH()` tea.Cmd in `app.go`
- Call `sshclient.Dial()` → wraps result in RemoteFS
- Assign to `m.remotePane.FS`
- Show spinner while dialing
- **Est. 1–2 hours**

**[#2] Host picker screen:**
- Add `hostPickerPane` to Model
- Route between screens (ScreenBrowsing vs ScreenHostPicker)
- Render & handle keys (↑/↓/Enter)
- Trigger `connectSSH()` on selection
- **Est. 30–45 min**

**[#3] Polish & theming:**
- Dark background (ANSI 0)
- Connection spinner
- Footer key-hints
- **Est. 30–60 min**

Once these three are done, the MVP is complete and functional for the user's original use case (moving podcast files from wintermute).

### Issues #4+ (Post-MVP)

These are enhancements and deferred scope. Start with #1–3 first.

---

## Common Gotchas & Debugging

### "Unknown message: struct { ID int; Filename string; Total int64 }"

This is the transfer-started message type. It's benign — the UI doesn't need to handle it, workers send it to announce the transfer. If you see this in logs, it's not an error.

### Transfer appears queued but never runs

Check:
- `jobsCh` is passed to Model (see `main.go`)
- Workers are started before `tea.NewProgram()` (see `main.go`)
- `enqueueCopy()` sends to `m.jobsCh` (see `app.go`)
- Transfer message types are imported from `transfer` package (see imports in `app.go`)

### UI freezes during SSH connect

The SSH dial runs as a `tea.Cmd` in its own goroutine. The UI should stay responsive. If it freezes:
- Check that `sshclient.Dial()` isn't blocking on something (key passphrase prompt, network timeout)
- Add a timeout to the SSH dial
- Check logs in `/tmp/exfil.log`

### Remote pane shows nothing after "connecting"

Likely causes:
- SSH auth failed silently → check logs
- RemoteFS not wired up correctly → check `sshConnectedMsg` handler in `app.go`
- SFTP listdir failed on remote path → check that path exists on remote

---

## Key Design Decisions (Why It's Built This Way)

| Decision | Rationale |
|----------|-----------|
| **Bubbletea + Lipgloss** | Elm architecture is perfect for TUI state machines. Lipgloss handles styling. |
| **CSP concurrency (no mutexes)** | Simpler, safer, easier to reason about. Workers & UI never fight over state. |
| **FileSystem interface** | Single pane code works for local or remote. Swaps at config time, not runtime. |
| **3 concurrent workers** | Good balance: enough parallelism, not too many goroutines. Bounded by channel recv. |
| **SSH-agent first, keys second** | Matches user's existing SSH setup. No extra secrets to manage. |
| **YAML config (not JSON)** | Supports comments. Humans can edit `~/.config/exfil/hosts.yaml` by hand. |
| **No password prompts** | MVP requirement: set up keys once, use many times. Agent or key files only. |

---

## If You Get Stuck

1. **Check CLAUDE.md** — More detailed implementation notes, patterns, pseudocode
2. **Check GitHub issues** — The issues have acceptance criteria and design context
3. **Read the code** — It's well-structured and intentional. Comments explain the "why"
4. **Check logs** — `/tmp/exfil.log` has detailed errors from workers
5. **Test locally first** — Verify local copy works before trying SSH

---

## Success Criteria for MVP Completion

When these are true, the MVP is done:

- [ ] App starts and shows host picker screen
- [ ] User selects wintermute from saved hosts
- [ ] SSH connects (spinner shown during dial)
- [ ] Remote pane lists real files from `/home/bruce/podcasts/output` on wintermute
- [ ] User can mark audio files and press 'c' to queue download
- [ ] Transfer queue shows progress bars
- [ ] Files appear in local destination pane as they complete
- [ ] `sha256sum` of downloaded files matches source
- [ ] No crashes on disconnect or interrupt

---

## Next Steps (For You, Right Now)

1. Pick up **issue #1** (SSH wiring) — it unblocks #2 and #3
2. Read `internal/sshclient/client.go` to understand the dial flow
3. Read `internal/ui/app.go` to understand where to add `connectSSH()`
4. Refer to the pseudocode in CLAUDE.md for the rough implementation
5. Wire it up, test locally first (should still work), then test against wintermute
6. Once #1 works, #2 and #3 are straightforward plumbing

Good luck! This is a well-scoped project with clean architecture. You've got this.
