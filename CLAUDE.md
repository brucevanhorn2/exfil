# exfil — Implementation Notes

## Project Goal

A cyberpunk-themed terminal UI SCP/SFTP client for Linux. Replace `scp` and bloated GUI clients with a fast, local, no-account TUI written in Go.

## MVP Status (as of tonight)

### Completed (M1-M4 partial)

- ✅ Dual-pane local file browser with navigation
- ✅ File selection and marking
- ✅ Transfer queue pane with progress bars
- ✅ Concurrent file copy (worker pool, 3 workers, bounded)
- ✅ Progress tracking with speed calculation
- ✅ Cyberpunk theming (dark, color-coded, lipgloss borders)
- ✅ Filesystem abstraction (LocalFS, RemoteFS ready but not wired)
- ✅ Config/site manager foundation (config.go loads/saves hosts.yaml)
- ✅ SSH client foundation (sshclient.go handles agent auth + key fallback)
- ✅ Transfer engine ready for cross-filesystem ops (RunWithFS accepting src/dst filesystems)

**What works end-to-end:**
1. Start the app: `./exfil`
2. Browse local files with arrow keys
3. Mark files with space
4. Press 'c' to copy (queues to transfer queue)
5. Watch progress bars in real time
6. Files transfer concurrently (up to 3 at once)

### Not yet complete (M3, M4 final)

**M3 (SSH/SFTP + Site Manager):**
- Need to wire SSH connection flow (see below)
- Site manager screen (host picker) is stubbed but not integrated
- Remote pane should use RemoteFS once SSH connects

**M4 Final (Polish):**
- Footer key-hints instead of inline status
- Animation/spinner during SSH dial
- Full theming pass (dark background ANSI 0, not default)

## Critical Path to Complete MVP

### Step 1: Wire SSH connection in the UI (1-2 hours)

Edit `internal/ui/app.go`:

1. Add a screen state for host picker (already has ScreenHostPicker const)
2. In `Update`, when screen == ScreenBrowsing and user connects, dial SSH:
   ```go
   case "ctrl+s": // or similar trigger
       return m, m.connectSSH(hostname)
   ```
3. Implement `connectSSH()` tea.Cmd that calls `sshclient.Dial()`, returns `sshConnectedMsg`
4. Handle `sshConnectedMsg` by wrapping sftp.Client in RemoteFS and assigning to remotePane.FS

Example outline (pseudocode):
```go
func (m *Model) connectSSH(hostname string) tea.Cmd {
    return func() tea.Msg {
        host := config.Host{
            Hostname: hostname,
            User: os.Getenv("USER"),
            Port: 22,
        }
        sshClient, sftpClient, err := sshclient.Dial(host)
        if err != nil {
            return sshConnectedMsg{err: err}
        }
        return sshConnectedMsg{
            sftpClient: sftpClient,
            sshClient:  sshClient,
        }
    }
}
```

5. In transfer.copy.go, the copy logic already calls `RunWithFS(job, events, src, dst)`, so when one pane is remote, just pass RemoteFS as the source or dest.

### Step 2: Integrate host picker screen (~30 min)

Edit `internal/ui/app.go`:
1. Add hostPickerPane to Model
2. In View(), render hostPickerPane when screen == ScreenHostPicker
3. In Update(), handle arrow keys and Enter for the picker
4. On Enter, trigger connectSSH() for the selected host

The hostpicker.go file already exists with Load(), CurrentHost(), Up(), Down(), View().

### Step 3: Test with wintermute (30 min)

1. Configure a host in ~/.config/exfil/hosts.yaml:
   ```yaml
   hosts:
     - name: wintermute
       hostname: wintermute.local
       port: 22
       user: bruce
       remote_path: /home/bruce/podcasts/output
   ```
2. Start the app, pick wintermute from the host list
3. Wait for SSH connection (should show spinner)
4. Right pane should list podcast files
5. Select and press 'c' to download to local

## Code Patterns & Guidelines

### Concurrency (CSP discipline)

- Worker goroutines **only send messages, never touch Model**
- Model.Update is single-threaded, mutates state safely
- eventsCh is buffered (cap 64)
- jobsCh is buffered (cap 256)
- On transfer message, always return `waitForEvent(m.eventsCh)` to re-arm the subscription

### FileSystem interface

Both panes implement the same fsys.FileSystem interface. This eliminates code duplication:
- `ReadDir(path)` → sorted entries
- `Join(elem...)` → path.Join (POSIX) or filepath.Join (local)
- `Open/Create` → io.ReadCloser / io.WriteCloser
- `Stat(path)` → single Entry

### Transfer progress

ProgressWriter wraps io.Writer, throttles to ~6 msgs/sec, emits TransferProgressMsg. This keeps eventsCh from flooding on fast copies.

### Config (hosts.yaml)

Located at `~/.config/exfil/hosts.yaml`. Loaded via `config.Load()`, saved via `cfg.Save()`. YAML format supports comments.

### SSH auth

sshclient.Dial tries:
1. ssh-agent (via SSH_AUTH_SOCK environment variable)
2. Fallback identity files in ~/.ssh: id_ed25519, id_rsa, id_ecdsa (in that order)

No password/passphrase prompts.

## Known Limitations

- Only one SSH connection per session (could extend with connection pool, but MVP is single host at a time)
- Directories can't be copied (shows "not supported")
- No delete, rename, mkdir, view/edit
- No 1Password integration (explicitly deferred by user)
- Transfer cancellation not implemented (Ctrl+C kills the app, partial files left on disk)

## Files to Touch for Completion

1. **internal/ui/app.go** — Add host picker screen routing, SSH connect flow
2. **internal/ui/hostpicker.go** — Already stubbed, just hook up to app
3. **cmd/exfil/main.go** — Might need to pass additional state (currently OK)

## Testing

Run the local copy test (one-shot):
```bash
go run test-local-copy.go  # (if test file exists)
```

Or manually:
1. `./exfil`
2. Create test dirs: `mkdir -p /tmp/test/{src,dst}`
3. Put a file in src: `echo hi > /tmp/test/src/file.txt`
4. In app, navigate both panes to /tmp/test (left=src, right=dst)
5. Mark file, press 'c'
6. Watch progress bar, file should appear in dst/

## Performance Notes

- Transfer speed is IO-bound, not CPU-bound (progress is very fast for local disk, slower on LAN)
- 3 concurrent workers is a good balance; can increase if needed
- Progress message throttle (150ms) prevents UI refresh thrashing
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
