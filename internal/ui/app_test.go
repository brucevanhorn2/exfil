package ui

import (
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

	cmd := m.enqueueCopyDirection(m.localPane, m.remotePane)
	if cmd == nil {
		t.Fatal("expected a Cmd from enqueueCopyDirection")
	}
	cmd()

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
			m.setTransferDest(id, "remote")
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
	if len(m.confirmDeleteNames) != 1 || m.confirmDeleteNames[0] != "victim.txt" {
		t.Fatalf("expected confirmDeleteNames to be [victim.txt], got %v", m.confirmDeleteNames)
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
	m.confirmDeleteNames = []string{"keepme.txt"}
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
