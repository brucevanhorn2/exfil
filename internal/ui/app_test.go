package ui

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/bvanhorn/exfil/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
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
