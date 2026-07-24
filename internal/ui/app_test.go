package ui

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/bvanhorn/exfil/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
)

var errTestTransferFailed = errors.New("simulated transfer failure")

func testLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
}

// drainCmd repeatedly runs cmd and feeds its message back into m.Update,
// following any further Cmd it returns, until none remain. fileOpDoneMsg's
// handler returns a readDirCmd (off-UI-thread refresh) rather than calling
// Refresh() synchronously, so tests need to chain through it — the same
// pattern TestTransferDoneMsgRefreshesDestinationPane drives by hand for the
// transfer-completion path.
func drainCmd(m *Model, cmd tea.Cmd) *Model {
	for cmd != nil {
		msg := cmd()
		var newModel tea.Model
		newModel, cmd = m.Update(msg)
		m = newModel.(*Model)
	}
	return m
}

// drainBatch runs a Cmd expected to produce a tea.BatchMsg (as
// TransferDoneMsg's handler does), feeding each sub-command's result back
// into m.Update. Used where a test only cares that the model ends up
// consistent, not the exact readDirMsg plumbing (see
// TestTransferDoneMsgRefreshesDestinationPane for that finer-grained check).
func drainBatch(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		var newModel tea.Model
		newModel, _ = m.Update(msg)
		return newModel.(*Model)
	}
	for _, sub := range batch {
		var newModel tea.Model
		newModel, _ = m.Update(sub())
		m = newModel.(*Model)
	}
	return m
}

func TestNewModelDefaultsWhenConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)

	if m.loc.Pack() != "plain" {
		t.Errorf("expected default pack \"plain\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected default primary color %q, got %q", DefaultPrimaryColor, m.primaryColorHex)
	}
	if m.secondaryColorHex != DefaultSecondaryColor {
		t.Errorf("expected default secondary color %q, got %q", DefaultSecondaryColor, m.secondaryColorHex)
	}
}

func TestNewModelUsesSavedLingoAndColors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{Lingo: "keyboardcowboy", PrimaryColor: "#39FF14", SecondaryColor: "#3A3A4A"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)

	if m.loc.Pack() != "keyboardcowboy" {
		t.Errorf("expected pack \"keyboardcowboy\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != "#39FF14" {
		t.Errorf("expected primary color #39FF14, got %q", m.primaryColorHex)
	}
}

func TestNewModelFallsBackOnInvalidStoredColor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{PrimaryColor: "not-a-color"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)

	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected fallback to default primary color for invalid stored value, got %q", m.primaryColorHex)
	}
}

// TestHandleSettingsKeyEnterAbortsSaveOnConfigLoadFailure guards against a
// data-loss bug: if hosts.yaml is present but fails to parse when the
// Settings screen saves, handleSettingsKey must NOT call cfg.Save() with a
// freshly-zeroed Config, which would silently wipe the existing Hosts list.
// It must instead abort and surface an error, matching HostFormPane.Save().
func TestHandleSettingsKeyEnterAbortsSaveOnConfigLoadFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.settingsPane.ResetFromConfig(m.loc.Pack(), m.primaryColorHex, m.secondaryColorHex)
	m.screen = ScreenSettings

	// Corrupt hosts.yaml after the Model has already loaded it once, so
	// config.Load() fails specifically inside the settings-save path.
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("hosts: [this is not valid: yaml\n")
	if err := os.WriteFile(p, corrupt, 0600); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleSettingsKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := newModel.(*Model)

	if m2.screen != ScreenBrowsing {
		t.Errorf("expected screen to return to Browsing even on save failure, got %v", m2.screen)
	}
	if m2.statusMsg == "" || m2.statusMsg == "Ready." {
		t.Errorf("expected an error status message, got %q", m2.statusMsg)
	}

	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(corrupt) {
		t.Errorf("hosts.yaml was overwritten despite config.Load() failure; got %q, want unchanged %q", after, corrupt)
	}
}

// TestTransferDoneMsgRefreshesDestinationPane guards against a regression of
// the "destination pane doesn't update after a transfer" bug: once a
// transfer.TransferDoneMsg arrives, the pane it copied into should be
// re-listed so the new file shows up without the user navigating away and
// back.
func TestTransferDoneMsgRefreshesDestinationPane(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// testMode=true: this test exercises a local-to-local transfer without a
	// real SSH connection, which enqueueCopyDirection only permits when
	// either connected or in test mode.
	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir

	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := m.remotePane.Refresh(); err != nil {
		t.Fatal(err)
	}
	if len(m.remotePane.Entries) != 0 {
		t.Fatalf("expected empty destination pane before transfer, got %v", m.remotePane.Entries)
	}

	// Enqueue the push (local -> remote) via the real code path; the
	// returned Cmd is just a function, safe to run synchronously here.
	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	cmd()

	// enqueueFileCopy sent a transferQueuedMsg to register the transfer in
	// the queue pane/transferDest before handing the job to the worker
	// pool — drain and process it the same way Update()'s real
	// waitForEvent subscription loop would, or the "unblock" send below
	// would deadlock on eventsCh's buffer already being full.
	select {
	case qmsg := <-m.eventsCh:
		m.Update(qmsg)
	default:
		t.Fatal("expected a transferQueuedMsg on eventsCh")
	}

	var job transfer.Job
	select {
	case job = <-m.jobsCh:
	default:
		t.Fatal("expected a job to be queued on jobsCh")
	}

	// Simulate the worker pool having already written the file (this test
	// exercises the post-completion refresh, not the copy itself).
	if err := os.WriteFile(filepath.Join(dstDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	// Unblock waitForEvent's channel receive so it's safe to run every Cmd
	// in the returned batch synchronously.
	m.eventsCh <- nil

	_, batchCmd := m.Update(transfer.TransferDoneMsg{ID: job.ID})
	if batchCmd == nil {
		t.Fatal("expected a Cmd after TransferDoneMsg")
	}

	msg := batchCmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}

	var found bool
	for _, sub := range batch {
		result := sub()
		rd, ok := result.(readDirMsg)
		if !ok {
			continue
		}
		found = true
		if rd.pane != "remote" {
			t.Errorf("expected refresh for the remote (destination) pane, got %q", rd.pane)
		}
		if rd.err != nil {
			t.Fatalf("unexpected error refreshing destination pane: %v", rd.err)
		}
		m.Update(rd)
	}
	if !found {
		t.Fatal("expected a readDirMsg among the batched commands")
	}

	names := make([]string, len(m.remotePane.Entries))
	for i, e := range m.remotePane.Entries {
		names[i] = e.Name
	}
	if len(names) != 1 || names[0] != "file.txt" {
		t.Errorf("expected destination pane to show the transferred file, got entries %v", names)
	}

	if _, stillTracked := m.transferDest[job.ID]; stillTracked {
		t.Errorf("expected transferDest entry for job %d to be cleared after completion", job.ID)
	}
}

// TestTransferDoneMsgClearsSourceSelection verifies that once a marked
// file's transfer succeeds, its checkmark is cleared in the pane it was
// copied *from* — but only that file; other marks are left untouched.
func TestTransferDoneMsgClearsSourceSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// eventsCh needs room for both files' transferQueuedMsg (sent
	// synchronously, back to back, by cmd() below) plus the "unblock" send
	// further down — a buffer of 1 would deadlock on the second
	// transferQueuedMsg send since nothing is draining concurrently here.
	m := NewModel(make(chan tea.Msg, 4), make(chan transfer.Job, 2), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir

	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "other.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := m.remotePane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// Mark both files, as if the user had multi-selected them before pushing.
	m.localPane.Selected["file.txt"] = true
	m.localPane.Selected["other.txt"] = true

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	cmd()

	// Drain both transferQueuedMsg (one per file) through Update() so
	// transferDest is actually populated — TransferDoneMsg's handler below
	// looks up srcPane/filename there to decide what to clear.
	if n := drainEvents(m); n != 2 {
		t.Fatalf("expected 2 transferQueuedMsg (file.txt, other.txt), got %d", n)
	}

	var jobs []transfer.Job
	for i := 0; i < 2; i++ {
		select {
		case job := <-m.jobsCh:
			jobs = append(jobs, job)
		default:
			t.Fatalf("expected 2 queued jobs, got %d", i)
		}
	}

	// Simulate only the job for file.txt completing; other.txt's transfer
	// hasn't reported back yet.
	var doneJob transfer.Job
	for _, job := range jobs {
		if job.Filename == "file.txt" {
			doneJob = job
		}
	}

	if err := os.WriteFile(filepath.Join(dstDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	m.eventsCh <- nil // unblock waitForEvent so the returned batch is safe to run synchronously

	_, batchCmd := m.Update(transfer.TransferDoneMsg{ID: doneJob.ID})
	_ = drainBatch(m, batchCmd)

	if m.localPane.Selected["file.txt"] {
		t.Error("expected file.txt's mark to be cleared after its transfer succeeded")
	}
	if !m.localPane.Selected["other.txt"] {
		t.Error("expected other.txt to remain marked; its transfer hasn't completed")
	}
}

// TestTransferErrorMsgLeavesSourceSelectionMarked verifies that a failed
// transfer does NOT clear the source file's mark, so the user can see what
// failed and retry without re-marking it.
func TestTransferErrorMsgLeavesSourceSelectionMarked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir

	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	m.localPane.Selected["file.txt"] = true

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	cmd()

	var job transfer.Job
	select {
	case job = <-m.jobsCh:
	default:
		t.Fatal("expected a job to be queued on jobsCh")
	}

	m.Update(transfer.TransferErrorMsg{ID: job.ID, Err: errTestTransferFailed})

	if !m.localPane.Selected["file.txt"] {
		t.Error("expected file.txt to remain marked after a failed transfer")
	}
}

// TestEnqueueCopyDirectionBlocksDisconnectedRemote is a regression test for
// a bug where the remote pane defaulted to browsing the local filesystem
// before any SSH connection was made — confusing when the local and remote
// users happen to share a username. Outside test mode, with no connection,
// a transfer touching the remote pane must be refused rather than silently
// copying to/from the local disk underneath it.
func TestEnqueueCopyDirectionBlocksDisconnectedRemote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir

	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// The not-connected guard now runs synchronously (part of fixing the
	// enqueueCopyDirection goroutine-mutation race), so it returns no Cmd
	// at all rather than a Cmd whose invocation sets the status message.
	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd != nil {
		t.Error("expected no Cmd while disconnected")
	}

	select {
	case job := <-m.jobsCh:
		t.Fatalf("expected no job to be queued while disconnected, got %+v", job)
	default:
	}

	if m.statusMsg == "" || m.statusMsg == "Ready." {
		t.Errorf("expected a not-connected status message, got %q", m.statusMsg)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); !os.IsNotExist(err) {
		t.Errorf("expected no file to be written to the destination while disconnected, stat err = %v", err)
	}
}

// TestViewSetsDisconnectedRemoteEmptyMessage confirms the remote pane's
// EmptyMessage is only populated when neither connected nor in test mode,
// and is cleared as soon as either becomes true.
func TestViewSetsDisconnectedRemoteEmptyMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.width, m.height = 100, 40

	m.View()
	if m.remotePane.EmptyMessage == "" {
		t.Error("expected remotePane.EmptyMessage to be set while disconnected and not in test mode")
	}

	m.connected = true
	m.View()
	if m.remotePane.EmptyMessage != "" {
		t.Error("expected remotePane.EmptyMessage to be cleared once connected")
	}

	m.connected = false
	m.testMode = true
	m.View()
	if m.remotePane.EmptyMessage != "" {
		t.Error("expected remotePane.EmptyMessage to be cleared in test mode")
	}
}

// TestTransferDestConcurrentAccess is a regression test for a data race:
// setTransferDest is called from enqueueCopyDirection's tea.Cmd goroutine,
// popTransferDest from Update()'s goroutine. A plain map has no protection
// against that — a concurrent map write racing a read/delete is a fatal,
// unrecoverable Go runtime error, not just a benign race. Run with -race to
// verify the mutex actually guards every access.
func TestTransferDestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(2 * n)

	for i := 0; i < n; i++ {
		id := i
		go func() {
			defer wg.Done()
			m.setTransferDest(id, "remote", "local", "file.txt")
		}()
		go func() {
			defer wg.Done()
			m.popTransferDest(id)
		}()
	}

	wg.Wait()
}

// TestHandleBrowsingKeyDeleteFlow drives the full d -> confirm(y) -> refresh
// path (issue #4): pressing "d" on the file under the cursor should move to
// ScreenConfirmDelete, and confirming should remove the file from disk and
// re-list the pane.
func TestHandleBrowsingKeyDeleteFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "victim.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = newModel.(*Model)

	if m.screen != ScreenConfirmDelete {
		t.Fatalf("expected ScreenConfirmDelete after 'd', got %v", m.screen)
	}
	if len(m.confirmDeleteTargets) != 1 || m.confirmDeleteTargets[0].Name != "victim.txt" || m.confirmDeleteTargets[0].IsDir {
		t.Fatalf("expected confirmDeleteTargets to be [{victim.txt false}], got %v", m.confirmDeleteTargets)
	}

	newModel, cmd := m.handleConfirmDeleteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = newModel.(*Model)
	if m.screen != ScreenBrowsing {
		t.Errorf("expected to return to ScreenBrowsing after confirming delete, got %v", m.screen)
	}
	if cmd == nil {
		t.Fatal("expected a Cmd from handleConfirmDeleteKey")
	}

	m = drainCmd(m, cmd)

	if _, err := os.Stat(filepath.Join(localDir, "victim.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected victim.txt to be deleted, stat err = %v", err)
	}
	if len(m.localPane.Entries) != 0 {
		t.Errorf("expected pane to be refreshed with no entries, got %v", m.localPane.Entries)
	}
}

// TestHandleBrowsingKeyDeleteRecursiveFlow drives d -> confirm(y) on a
// non-empty marked directory (issue #15): the whole tree should be removed
// via RemoveAll, and the confirm screen should have escalated to the
// recursive header/message.
func TestHandleBrowsingKeyDeleteRecursiveFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	sub := filepath.Join(localDir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = newModel.(*Model)

	if m.screen != ScreenConfirmDelete {
		t.Fatalf("expected ScreenConfirmDelete after 'd', got %v", m.screen)
	}
	if len(m.confirmDeleteTargets) != 1 || m.confirmDeleteTargets[0].Name != "sub" || !m.confirmDeleteTargets[0].IsDir {
		t.Fatalf("expected confirmDeleteTargets to be [{sub true}], got %v", m.confirmDeleteTargets)
	}
	if !m.confirmDeleteRecursive {
		t.Error("expected confirmDeleteRecursive to be true when a directory is marked")
	}

	newModel, cmd := m.handleConfirmDeleteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = newModel.(*Model)
	if cmd == nil {
		t.Fatal("expected a Cmd from handleConfirmDeleteKey")
	}
	m = drainCmd(m, cmd)

	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatalf("expected sub (and everything inside it) to be deleted, stat err = %v", err)
	}
	if len(m.localPane.Entries) != 0 {
		t.Errorf("expected pane to be refreshed with no entries, got %v", m.localPane.Entries)
	}
}

// TestHandleBrowsingKeyDeleteMixedSelectionFlow marks one plain file and one
// non-empty directory together (issue #15's "mixed selection" decision):
// a single d/confirm should remove both, using Remove for the file and
// RemoveAll for the directory.
func TestHandleBrowsingKeyDeleteMixedSelectionFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	sub := filepath.Join(localDir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "file.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// Mark both entries (cursor starts on "file.txt" — dirs sort first, so
	// "sub" is entry 0 and "file.txt" is entry 1).
	m.localPane.ToggleSelect()
	m.localPane.Down()
	m.localPane.ToggleSelect()

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = newModel.(*Model)

	if len(m.confirmDeleteTargets) != 2 {
		t.Fatalf("expected both marked entries in confirmDeleteTargets, got %v", m.confirmDeleteTargets)
	}

	newModel, cmd := m.handleConfirmDeleteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = newModel.(*Model)
	if cmd == nil {
		t.Fatal("expected a Cmd from handleConfirmDeleteKey")
	}
	drainCmd(m, cmd)

	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("expected sub to be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, "file.txt")); !os.IsNotExist(err) {
		t.Errorf("expected file.txt to be deleted, stat err = %v", err)
	}
}

// TestHandleBrowsingKeyDeleteNonRecursiveMessageForFiles confirms plain
// files still get the original (non-escalated) confirm wording — the
// recursive header/message must only trigger when a directory is involved.
func TestHandleBrowsingKeyDeleteNonRecursiveMessageForFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = newModel.(*Model)

	if m.confirmDeleteRecursive {
		t.Error("expected confirmDeleteRecursive to be false for a file-only delete")
	}
}

// TestHandleConfirmDeleteKeyCancel confirms "n"/"esc" abort without touching
// the filesystem.
func TestHandleConfirmDeleteKeyCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "keepme.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}
	m.confirmDeleteTargets = []deleteTarget{{Name: "keepme.txt"}}
	m.confirmDeletePaneName = "local"
	m.screen = ScreenConfirmDelete

	newModel, cmd := m.handleConfirmDeleteKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = newModel.(*Model)

	if m.screen != ScreenBrowsing {
		t.Errorf("expected ScreenBrowsing after cancel, got %v", m.screen)
	}
	if cmd != nil {
		t.Error("expected no Cmd after cancelling delete")
	}
	if _, err := os.Stat(filepath.Join(localDir, "keepme.txt")); err != nil {
		t.Errorf("expected keepme.txt to survive cancel, stat err = %v", err)
	}
}

// TestHandleBrowsingKeyRenameFlow drives r -> edit -> Enter, verifying the
// file is renamed on disk and the pane refreshed.
func TestHandleBrowsingKeyRenameFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "old.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = newModel.(*Model)

	if m.screen != ScreenPrompt || m.promptMode != "rename" {
		t.Fatalf("expected ScreenPrompt/rename after 'r', got screen=%v mode=%q", m.screen, m.promptMode)
	}
	if m.promptPane.Value() != "old.txt" {
		t.Errorf("expected prompt pre-filled with old.txt, got %q", m.promptPane.Value())
	}

	m.promptPane.Input.SetValue("new.txt")
	newModel, cmd := m.handlePromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if m.screen != ScreenBrowsing {
		t.Errorf("expected ScreenBrowsing after rename Enter, got %v", m.screen)
	}
	if cmd == nil {
		t.Fatal("expected a Cmd from handlePromptKey")
	}
	drainCmd(m, cmd)

	if _, err := os.Stat(filepath.Join(localDir, "old.txt")); !os.IsNotExist(err) {
		t.Errorf("expected old.txt to be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, "new.txt")); err != nil {
		t.Errorf("expected new.txt to exist, stat err = %v", err)
	}
}

// TestHandleBrowsingKeyRenameRefusesExistingTarget guards against silent
// data loss: os.Rename (LocalFS) would otherwise overwrite an existing
// destination with no warning, unlike delete which requires explicit Y/N
// confirmation for the same feature.
func TestHandleBrowsingKeyRenameRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "old.txt"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "taken.txt"), []byte("do not overwrite"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = newModel.(*Model)

	m.promptPane.Input.SetValue("taken.txt")
	newModel, cmd := m.handlePromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)
	if cmd == nil {
		t.Fatal("expected a Cmd from handlePromptKey")
	}
	m = drainCmd(m, cmd)

	got, err := os.ReadFile(filepath.Join(localDir, "taken.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "do not overwrite" {
		t.Errorf("taken.txt was overwritten by rename; got %q", got)
	}
	if _, err := os.Stat(filepath.Join(localDir, "old.txt")); err != nil {
		t.Errorf("expected old.txt to still exist since rename was refused, stat err = %v", err)
	}
	if m.statusMsg == "" || m.statusMsg == "Ready." {
		t.Errorf("expected an error status message, got %q", m.statusMsg)
	}
}

// TestHandleBrowsingKeyRenameNoOpDoesNotError guards against a rough edge
// found in review: submitting the rename prompt without changing the name
// would otherwise hit the same destination-exists check as a real collision
// (Stat finds the file being "renamed" since it's the same path), showing a
// confusing "already exists" error for what should be a harmless no-op.
func TestHandleBrowsingKeyRenameNoOpDoesNotError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "same.txt"), []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}
	m.statusMsg = "Ready."

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = newModel.(*Model)

	// Don't touch m.promptPane's value — submit exactly the pre-filled name.
	newModel, cmd := m.handlePromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if m.screen != ScreenBrowsing {
		t.Errorf("expected ScreenBrowsing after no-op rename Enter, got %v", m.screen)
	}
	if cmd != nil {
		t.Error("expected no Cmd for a no-op rename (nothing to do, no I/O to perform)")
	}
	if m.statusMsg != "Ready." {
		t.Errorf("expected no error status message for a no-op rename, got %q", m.statusMsg)
	}
	got, err := os.ReadFile(filepath.Join(localDir, "same.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unchanged" {
		t.Errorf("expected same.txt to be untouched, got %q", got)
	}
}

// TestHandleBrowsingKeyMkdirFlow drives m -> type a name -> Enter, verifying
// the directory is created on disk and the pane refreshed.
func TestHandleBrowsingKeyMkdirFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	localDir := t.TempDir()

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = localDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = newModel.(*Model)

	if m.screen != ScreenPrompt || m.promptMode != "mkdir" {
		t.Fatalf("expected ScreenPrompt/mkdir after 'm', got screen=%v mode=%q", m.screen, m.promptMode)
	}

	m.promptPane.Input.SetValue("newdir")
	newModel, cmd := m.handlePromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*Model)

	if cmd == nil {
		t.Fatal("expected a Cmd from handlePromptKey")
	}
	drainCmd(m, cmd)

	info, err := os.Stat(filepath.Join(localDir, "newdir"))
	if err != nil {
		t.Fatalf("expected newdir to exist, stat err = %v", err)
	}
	if !info.IsDir() {
		t.Error("expected newdir to be a directory")
	}
}

// TestHandleBrowsingKeyFileOpsBlockDisconnectedRemote guards d/r/m the same
// way TestEnqueueCopyDirectionBlocksDisconnectedRemote guards transfers: none
// of them should touch the remote pane before a real SSH connection exists.
func TestHandleBrowsingKeyFileOpsBlockDisconnectedRemote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)
	m.localPane.SetFocus(false)
	m.remotePane.SetFocus(true)

	for _, key := range []string{"d", "r", "m"} {
		newModel, cmd := m.handleBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = newModel.(*Model)
		if m.screen != ScreenBrowsing {
			t.Errorf("key %q: expected to stay on ScreenBrowsing while disconnected, got %v", key, m.screen)
		}
		if cmd != nil {
			t.Errorf("key %q: expected no Cmd while disconnected", key)
		}
	}
}

// drainEvents processes every message currently buffered on m.eventsCh
// through m.Update(), the same way Update()'s real waitForEvent
// subscription loop would one at a time — used by tests that enqueue a
// recursive copy, since transferQueuedMsg/transferQueueErrorMsg only take
// effect once Update() has handled them (all Model mutation happens there,
// never in the walker goroutine itself).
func drainEvents(m *Model) int {
	n := 0
	for len(m.eventsCh) > 0 {
		m.Update(<-m.eventsCh)
		n++
	}
	return n
}

// TestEnqueueCopyDirectionRecursiveCopy drives a directory copy (issue #6):
// marking a non-empty directory and pushing it across should mirror its
// structure on the destination (via MkdirAll, verified directly on disk)
// and enqueue one job per file with paths that preserve that structure.
func TestEnqueueCopyDirectionRecursiveCopy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	sub := filepath.Join(srcDir, "sub")
	nested := filepath.Join(sub, "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "file1.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file2.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 64), make(chan transfer.Job, 64), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// "sub" is the only entry; mark it (cursor starts there).
	m.localPane.ToggleSelect()

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	finalMsg := cmd()

	// MkdirAll runs synchronously during the walk, independent of the
	// worker pool that actually copies file bytes, so the mirrored
	// structure should already exist on disk.
	if info, err := os.Stat(filepath.Join(dstDir, "sub")); err != nil || !info.IsDir() {
		t.Fatalf("expected dstDir/sub to exist as a directory, err=%v", err)
	}
	if info, err := os.Stat(filepath.Join(dstDir, "sub", "nested")); err != nil || !info.IsDir() {
		t.Fatalf("expected dstDir/sub/nested to exist as a directory, err=%v", err)
	}

	// Since files were enqueued (totalFiles > 0), the Cmd deliberately does
	// NOT also refresh the destination pane itself — the usual per-file
	// TransferDoneMsg refresh already covers that, and having both would be
	// two independent, unordered refreshes racing each other. See
	// TestEnqueueCopyDirectionEmptyDirectoryTriggersRefresh for the case
	// where nothing else will ever trigger that refresh.
	if finalMsg != nil {
		t.Errorf("expected no extra refresh message when files were enqueued, got %#v", finalMsg)
	}

	queuedCount := drainEvents(m)
	if queuedCount != 2 {
		t.Fatalf("expected 2 transferQueuedMsg (file1.txt, nested/file2.txt), got %d", queuedCount)
	}
	if len(m.queuePane.Transfers) != 2 {
		t.Errorf("expected 2 queue pane entries, got %d", len(m.queuePane.Transfers))
	}

	gotPaths := map[string]string{}
	for len(m.jobsCh) > 0 {
		job := <-m.jobsCh
		gotPaths[job.SourcePath] = job.DestPath
	}

	wantSrc1 := filepath.Join(sub, "file1.txt")
	wantDst1 := filepath.Join(dstDir, "sub", "file1.txt")
	wantSrc2 := filepath.Join(nested, "file2.txt")
	wantDst2 := filepath.Join(dstDir, "sub", "nested", "file2.txt")
	if gotPaths[wantSrc1] != wantDst1 {
		t.Errorf("expected job %s -> %s, got -> %q", wantSrc1, wantDst1, gotPaths[wantSrc1])
	}
	if gotPaths[wantSrc2] != wantDst2 {
		t.Errorf("expected job %s -> %s, got -> %q", wantSrc2, wantDst2, gotPaths[wantSrc2])
	}
}

// TestEnqueueCopyDirectionEmptyDirectoryTriggersRefresh covers the case
// enqueueCopyDirection's eager end-of-walk refresh actually exists for:
// copying an entirely empty directory enqueues zero files, so nothing would
// ever trigger the usual per-file TransferDoneMsg refresh — the Cmd must
// refresh the destination pane itself instead.
func TestEnqueueCopyDirectionEmptyDirectoryTriggersRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(srcDir, "emptydir"), 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 64), make(chan transfer.Job, 64), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	m.localPane.ToggleSelect()

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	finalMsg := cmd()

	if finalMsg == nil {
		t.Fatal("expected the eager refresh message since no files were enqueued")
	}
	m.Update(finalMsg)

	found := false
	for _, e := range m.remotePane.Entries {
		if e.Name == "emptydir" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected destination pane to show 'emptydir' after the walk, got %v", m.remotePane.Entries)
	}
	if len(m.queuePane.Transfers) != 0 {
		t.Errorf("expected no queued transfers for an empty directory, got %v", m.queuePane.Transfers)
	}
}

// TestEnqueueCopyDirectionMixedSelectionCopy marks one plain file and one
// non-empty directory together (issue #6's "mixed selection" decision): a
// single push should enqueue jobs for both.
func TestEnqueueCopyDirectionMixedSelectionCopy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	sub := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "plain.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 64), make(chan transfer.Job, 64), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// Mark both entries (dirs sort first: "sub" is entry 0, "plain.txt" 1).
	m.localPane.ToggleSelect()
	m.localPane.Down()
	m.localPane.ToggleSelect()

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	cmd()
	drainEvents(m)

	gotFiles := map[string]bool{}
	for len(m.jobsCh) > 0 {
		job := <-m.jobsCh
		gotFiles[job.Filename] = true
	}
	if !gotFiles["child.txt"] {
		t.Error("expected a job for sub/child.txt")
	}
	if !gotFiles["plain.txt"] {
		t.Error("expected a job for plain.txt")
	}
}

// TestEnqueueCopyDirectionSkipsUnreadableSubtreeContinues guards the "skip
// and continue" decision for issue #6: a ReadDir failure on one marked
// directory shouldn't abort copying other marked entries.
func TestEnqueueCopyDirectionSkipsUnreadableSubtreeContinues(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	baddir := filepath.Join(srcDir, "baddir")
	if err := os.Mkdir(baddir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baddir, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(baddir, 0000); err != nil {
		t.Fatal(err)
	}
	// Restore permissions so t.TempDir()'s cleanup can remove baddir.
	defer func() {
		if err := os.Chmod(baddir, 0755); err != nil {
			t.Logf("failed to restore baddir permissions: %v", err)
		}
	}()

	if err := os.WriteFile(filepath.Join(srcDir, "goodfile.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 64), make(chan transfer.Job, 64), testLogger(), true)
	m.localPane.FS = fsys.LocalFS{}
	m.localPane.Cwd = srcDir
	m.remotePane.FS = fsys.LocalFS{}
	m.remotePane.Cwd = dstDir
	if err := m.localPane.Refresh(); err != nil {
		t.Fatal(err)
	}

	// Mark both entries ("baddir" sorts before "goodfile.txt").
	m.localPane.ToggleSelect()
	m.localPane.Down()
	m.localPane.ToggleSelect()

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	cmd()
	drainEvents(m)

	if m.statusMsg == "" || m.statusMsg == "Ready." {
		t.Error("expected an error status message for the unreadable subtree")
	}

	gotFiles := map[string]bool{}
	for len(m.jobsCh) > 0 {
		job := <-m.jobsCh
		gotFiles[job.Filename] = true
	}
	if !gotFiles["goodfile.txt"] {
		t.Error("expected goodfile.txt to still be enqueued despite baddir's failure")
	}
}

// TestAllocateTransferIDConcurrentAccess is a regression test for a data
// race: enqueueFileCopy calls allocateTransferID from the directory-walk
// goroutine, concurrently with any other goroutine doing the same (e.g. two
// copy keypresses in a row). Run with -race to verify the mutex actually
// guards m.nextID, and that every allocated ID is unique.
func TestAllocateTransferIDConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger(), false)

	const n = 200
	ids := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			ids[i] = m.allocateTransferID()
		}()
	}
	wg.Wait()

	seen := make(map[int]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("allocateTransferID returned duplicate ID %d", id)
		}
		seen[id] = true
	}
}
