package ui

import (
	"errors"
	"log"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/sshclient"
	"github.com/bvanhorn/exfil/internal/transfer"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Screen string

const (
	ScreenBrowsing      Screen = "browsing"
	ScreenHostPicker    Screen = "hostpicker"
	ScreenAddHost       Screen = "addhost"
	ScreenAbout         Screen = "about"
	ScreenSettings      Screen = "settings"
	ScreenPrompt        Screen = "prompt"
	ScreenConfirmDelete Screen = "confirmdelete"
)

// Messages
type readDirMsg struct {
	pane    string
	entries []fsys.Entry
	err     error
}

type sshConnectedMsg struct {
	host       config.Host
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	err        error
}

// Model is the root bubbletea model
type Model struct {
	width             int
	height            int
	screen            Screen
	theme             Theme
	loc               *i18n.Localizer
	primaryColorHex   string
	secondaryColorHex string
	localPane         *BrowserPane
	remotePane        *BrowserPane
	hostPicker        *HostPickerPane
	hostForm          *HostFormPane
	aboutPane         *AboutPane
	settingsPane      *SettingsPane
	queuePane         *QueuePane
	promptPane        *PromptPane
	confirmPane       *ConfirmPane
	statusMsg         string
	// nextID is allocated via allocateTransferID() rather than incremented
	// directly, guarded by nextIDMu — recursive directory copy enqueues
	// files from a background walker goroutine (internal/ui/copyops.go),
	// concurrently with Update()'s own goroutine, so a plain int++ would be
	// a data race.
	nextID   int
	nextIDMu sync.Mutex

	// promptMode/promptPaneName/promptOldName carry context for ScreenPrompt
	// between the browsing-screen key that opened it (r/m) and Enter's
	// handling in handlePromptKey — mirrors confirmDelete* below for delete.
	promptMode     string // "rename" or "mkdir"
	promptPaneName string // "local" or "remote": which BrowserPane the op targets
	promptOldName  string // only used for rename

	confirmDeleteTargets   []deleteTarget
	confirmDeletePaneName  string
	confirmDeleteRecursive bool
	// transferDest maps an in-flight transfer's ID to which pane it's
	// copying into/out of and which file, so TransferDoneMsg knows which
	// pane to refresh and, on success, which pane/filename to clear from
	// selection. Entries are removed once the transfer finishes or errors.
	// setTransferDest/popTransferDest are both only ever called from
	// Update()'s own goroutine today (via the transferQueuedMsg/
	// TransferDoneMsg/TransferErrorMsg cases), so transferDestMu is no
	// longer strictly required by a live cross-goroutine caller — kept as
	// cheap defense-in-depth against a future direct-mutation call site
	// (the same mistake the pre-#6 enqueueCopyDirection made) rather than
	// removed on the assumption that today's call sites are permanent.
	transferDest   map[int]transferInfo
	transferDestMu sync.Mutex

	eventsCh chan tea.Msg
	jobsCh   chan transfer.Job
	logger   *log.Logger

	// SSH connection state. Held so we can close cleanly and so the remote
	// pane's RemoteFS shares the single sftp client (safe for concurrent use).
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	connected  bool

	// connecting is true while an SSH dial is in flight; drives the spinner.
	spinner    spinner.Model
	connecting bool

	// testMode (the -t CLI flag) shows the local filesystem in the remote
	// pane too, without a real connection — for local-to-local transfer
	// testing. Outside test mode, the remote pane stays empty (no
	// navigation, no transfers) until a real SSH connection is made, so it
	// never gets mistaken for a live remote host.
	testMode bool
}

func NewModel(eventsCh chan tea.Msg, jobsCh chan transfer.Job, logger *log.Logger, testMode bool) *Model {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	localFS := fsys.LocalFS{}
	home, _ := localFS.Home()

	cfg, err := config.Load()
	if err != nil {
		logger.Printf("failed to load hosts.yaml for lingo/theme settings: %v", err)
		cfg = &config.Config{}
	}

	lingo := cfg.Lingo
	if lingo == "" {
		lingo = "plain"
	}
	loc := i18n.NewLocalizer(lingo)

	primaryColorHex := cfg.PrimaryColor
	if primaryColorHex == "" {
		primaryColorHex = DefaultPrimaryColor
	}
	primaryColor, err := parseHexColor(primaryColorHex)
	if err != nil {
		logger.Printf("invalid primary_color %q in hosts.yaml, using default: %v", primaryColorHex, err)
		primaryColorHex = DefaultPrimaryColor
		primaryColor = lipgloss.Color(DefaultPrimaryColor)
	}

	secondaryColorHex := cfg.SecondaryColor
	if secondaryColorHex == "" {
		secondaryColorHex = DefaultSecondaryColor
	}
	secondaryColor, err := parseHexColor(secondaryColorHex)
	if err != nil {
		logger.Printf("invalid secondary_color %q in hosts.yaml, using default: %v", secondaryColorHex, err)
		secondaryColorHex = DefaultSecondaryColor
		secondaryColor = lipgloss.Color(DefaultSecondaryColor)
	}

	theme := NewTheme(primaryColor, secondaryColor)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.PaneTitleFocus

	hostPicker := NewHostPickerPane()
	if err := hostPicker.Load(); err != nil {
		logger.Printf("failed to load hosts.yaml: %v", err)
	}
	hostForm := NewHostFormPane()
	aboutPane := NewAboutPane()
	settingsPane := NewSettingsPane()
	promptPane := NewPromptPane()

	m := &Model{
		screen:            ScreenBrowsing,
		theme:             theme,
		loc:               loc,
		primaryColorHex:   primaryColorHex,
		secondaryColorHex: secondaryColorHex,
		eventsCh:          eventsCh,
		jobsCh:            jobsCh,
		logger:            logger,
		localPane:         NewBrowserPane("local", localFS),
		remotePane:        NewBrowserPane("remote", fsys.LocalFS{}),
		hostPicker:        hostPicker,
		hostForm:          hostForm,
		aboutPane:         aboutPane,
		settingsPane:      settingsPane,
		promptPane:        promptPane,
		confirmPane:       &ConfirmPane{},
		queuePane:         NewQueuePane(),
		spinner:           sp,
		statusMsg:         loc.T("status_ready"),
		nextID:            1,
		transferDest:      make(map[int]transferInfo),
		testMode:          testMode,
	}

	m.localPane.Cwd = home
	m.localPane.SetFocus(true)
	m.remotePane.SetFocus(false)

	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		func() tea.Msg {
			if err := m.localPane.Refresh(); err != nil {
				return readDirMsg{pane: "local", err: err}
			}
			return readDirMsg{pane: "local", entries: m.localPane.Entries}
		},
		waitForEvent(m.eventsCh),
	}

	if m.testMode {
		// -t: refresh the remote pane too, even before an SSH connection is
		// made — it defaults to a LocalFS rooted at "/", which lets both
		// panes be used for local-to-local testing (see README). Once
		// connected, sshConnectedMsg's readDirCmd overwrites this with the
		// real remote listing. Outside test mode, the remote pane stays
		// empty until a real connection is made (see handleBrowsingKey and
		// View()), so it's never mistaken for a live remote host.
		cmds = append(cmds, func() tea.Msg {
			if err := m.remotePane.Refresh(); err != nil {
				return readDirMsg{pane: "remote", err: err}
			}
			return readDirMsg{pane: "remote", entries: m.remotePane.Entries}
		})
	}

	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Pane dimensions are (re)computed in View() every render from
		// m.width/m.height, so there's a single source of truth for layout.
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Route keys by the active screen. The host picker is a modal overlay
		// on top of the browsing view.
		if m.screen == ScreenHostPicker {
			return m.handleHostPickerKey(msg)
		}
		if m.screen == ScreenAddHost {
			return m.handleHostFormKey(msg)
		}
		if m.screen == ScreenAbout {
			return m.handleAboutKey(msg)
		}
		if m.screen == ScreenSettings {
			return m.handleSettingsKey(msg)
		}
		if m.screen == ScreenPrompt {
			return m.handlePromptKey(msg)
		}
		if m.screen == ScreenConfirmDelete {
			return m.handleConfirmDeleteKey(msg)
		}
		return m.handleBrowsingKey(msg)

	case spinner.TickMsg:
		// Keep the connect spinner animating only while a dial is in flight.
		// Re-arming the tick only when connecting avoids idle redraws.
		if m.connecting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case sshConnectedMsg:
		m.connecting = false
		if msg.err != nil {
			m.statusMsg = m.loc.T("status_connection_failed", msg.host.Name, msg.err)
			m.logger.Printf("ssh dial %s: %v", msg.host.Name, msg.err)
			return m, nil
		}
		// Wire the remote pane to a RemoteFS backed by the single sftp client.
		m.sshClient = msg.sshClient
		m.sftpClient = msg.sftpClient
		m.connected = true
		rfs := fsys.NewRemoteFS(msg.sftpClient)
		m.remotePane.FS = rfs
		m.remotePane.Title = msg.host.Name

		cwd := msg.host.RemotePath
		if cwd == "" {
			cwd, _ = rfs.Home()
		}
		m.remotePane.Cwd = cwd
		m.statusMsg = m.loc.T("status_connected", msg.host.User, msg.host.Hostname)
		// List the remote directory off the UI thread (network call).
		return m, readDirCmd("remote", rfs, cwd)

	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = m.loc.T("status_read_dir_error", msg.err)
		}
		switch msg.pane {
		case "local":
			m.localPane.SetEntries(msg.entries)
		case "remote":
			m.remotePane.SetEntries(msg.entries)
		}

	case transfer.TransferProgressMsg:
		// Worker sent a progress update. Update the queue pane and re-arm the subscription.
		// CRITICAL: Always return waitForEvent() after handling transfer messages,
		// or the subscription dies and we stop receiving progress.
		m.queuePane.UpdateTransfer(msg.ID, StatusRunning, msg.Done, msg.Total, msg.Speed, "")
		return m, waitForEvent(m.eventsCh)

	case transfer.TransferDoneMsg:
		// Transfer completed successfully.
		m.queuePane.UpdateTransfer(msg.ID, StatusDone, 0, 0, "", "")
		info, ok := m.popTransferDest(msg.ID)
		if !ok {
			return m, waitForEvent(m.eventsCh)
		}
		// Clear the file's mark in the pane it was copied from — this file
		// succeeded, so it no longer needs to stay selected. Other marked
		// files (still in flight or yet to complete) are untouched. If this
		// file came from a recursive directory copy, also feed it into its
		// directory's own group bookkeeping (completeGroupFile), which may
		// in turn clear the MARKED DIRECTORY's own selection — see
		// dirCopyGroup.
		m.paneByName(info.srcPane).ClearSelected(info.filename)
		completeGroupFile(info.group, false)
		dstPane := m.paneByName(info.destPane)
		// Re-list whichever directory the destination pane currently shows,
		// so the newly-arrived file appears without the user navigating
		// away and back.
		return m, tea.Batch(waitForEvent(m.eventsCh), readDirCmd(info.destPane, dstPane.FS, dstPane.Cwd))

	case transfer.TransferErrorMsg:
		// Transfer failed. Mark it as error and keep the error message visible.
		m.queuePane.UpdateTransfer(msg.ID, StatusError, 0, 0, "", msg.Err.Error())
		info, _ := m.popTransferDest(msg.ID)
		// Also marks this file's directory group (if any) as failed, so
		// the marked directory's own mark won't clear — see
		// completeGroupFile/dirCopyGroup.
		completeGroupFile(info.group, true)
		return m, waitForEvent(m.eventsCh)

	case transferQueuedMsg:
		// A recursive (or flat multi-select) copy discovered a file and
		// handed it to the worker pool off the UI thread; this is where
		// that registration actually lands in the queue pane/transferDest,
		// keeping all Model mutation inside Update() (see copyops.go).
		m.setTransferDest(msg.id, msg.destPane, msg.srcPane, msg.filename, msg.group)
		m.queuePane.AddTransfer(Transfer{
			ID:       msg.id,
			Filename: msg.filename,
			Status:   StatusQueued,
			Total:    msg.total,
		})
		return m, waitForEvent(m.eventsCh)

	case transferQueueErrorMsg:
		// A directory's MkdirAll/ReadDir failed while walking — reported
		// via statusMsg since it happened before any transfer.Job (and
		// queue pane row) existed for it to attach to. Per the "skip and
		// continue" decision for issue #6, the rest of the walk/copy
		// isn't aborted; this is purely informational.
		m.statusMsg = m.loc.T("status_copy_dir_error", msg.label, msg.err)
		if msg.group != nil {
			msg.group.failed = true
			maybeFinalizeGroup(msg.group)
		}
		return m, waitForEvent(m.eventsCh)

	case dirWalkDoneMsg:
		// The walk for one top-level marked directory has fully finished
		// discovering (and enqueueing) its files — see dirWalkDoneMsg's
		// comment in copyops.go for the ordering guarantee this relies on.
		msg.group.walkDone = true
		msg.group.discovered = msg.discovered
		maybeFinalizeGroup(msg.group)
		return m, waitForEvent(m.eventsCh)

	case fileOpDoneMsg:
		switch {
		case errors.Is(msg.err, errRenameTargetExists):
			m.statusMsg = m.loc.T("status_rename_exists")
		case msg.err != nil:
			m.statusMsg = m.loc.T(fileOpErrorKey(msg.action), msg.err)
		default:
			m.statusMsg = m.loc.T(fileOpSuccessKey(msg.action))
		}
		// Refresh regardless of error/success: a delete of several marked
		// files can partially succeed before hitting an error, so the
		// listing may be stale either way. Done via readDirCmd (off the UI
		// thread), matching TransferDoneMsg just above — Refresh() is a
		// network round-trip for the remote/SFTP pane and would otherwise
		// block the whole TUI on Update()'s goroutine.
		pane := m.paneByName(msg.pane)
		return m, readDirCmd(msg.pane, pane.FS, pane.Cwd)
	}

	return m, nil
}

// handleBrowsingKey handles keys in the dual-pane browsing screen.
func (m *Model) handleBrowsingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	active := m.remotePane
	if m.localPane.Focus {
		active = m.localPane
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.closeSSH()
		return m, tea.Quit
	case "s":
		// Open the Site Manager overlay to pick a host to connect to.
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = m.loc.T("status_hosts_load_error", err)
		}
		m.screen = ScreenHostPicker
	case "?":
		m.screen = ScreenAbout
	case "S":
		m.settingsPane.ResetFromConfig(m.loc.Pack(), m.primaryColorHex, m.secondaryColorHex)
		m.screen = ScreenSettings
	case "tab":
		newLocalFocus := !m.localPane.Focus
		m.localPane.SetFocus(newLocalFocus)
		m.remotePane.SetFocus(!newLocalFocus)
	case "up":
		active.Up()
	case "down":
		active.Down()
	case "right":
		// Push selected/current file(s) from local into remote.
		return m, m.enqueueCopyDirection(m.localPane, m.remotePane)
	case "left":
		// Pull selected/current file(s) from remote into local.
		return m, m.enqueueCopyDirection(m.remotePane, m.localPane)
	case "enter":
		if err := active.Enter(); err != nil {
			m.statusMsg = m.loc.T("error_prefix") + err.Error()
		}
	case "backspace":
		if err := active.Back(); err != nil {
			m.statusMsg = m.loc.T("error_prefix") + err.Error()
		}
	case " ":
		active.ToggleSelect()
	case "c":
		return m, m.enqueueCopy()
	case "d":
		if m.remoteBlocked(active) {
			m.statusMsg = m.loc.T("status_not_connected")
			return m, nil
		}
		files := active.GetSelectedFiles()
		if len(files) == 0 {
			if entry := active.CurrentFile(); entry != nil {
				files = []string{entry.Name}
			}
		}
		if len(files) == 0 {
			m.statusMsg = m.loc.T("status_no_files_selected")
			return m, nil
		}
		// IsDir is snapshotted here (from the already-loaded listing) and not
		// re-checked when "y" confirms — same accepted TOCTOU tradeoff as
		// rename's destination-exists check: a concurrent external change to
		// the target between marking and confirming is a narrow, essentially
		// theoretical race for a single-user desktop TUI, not worth an extra
		// Stat round-trip to defend against.
		targets := make([]deleteTarget, len(files))
		anyDir := false
		for i, name := range files {
			isDir := false
			if entry := active.EntryByName(name); entry != nil {
				isDir = entry.IsDir
			}
			targets[i] = deleteTarget{Name: name, IsDir: isDir}
			anyDir = anyDir || isDir
		}
		m.confirmDeleteTargets = targets
		m.confirmDeletePaneName = m.paneName(active)
		// A marked directory escalates to the stronger recursive wording —
		// deliberately with no upfront "is it actually empty?" check (that
		// would mean walking the tree just to decide, doubling round-trips
		// on a remote/SFTP pane before the user even sees the prompt), so
		// any marked directory gets the recursive warning even if it turns
		// out to be empty.
		m.confirmDeleteRecursive = anyDir
		msgKey := "confirm_delete_message"
		if anyDir {
			msgKey = "confirm_delete_recursive_message"
		}
		m.confirmPane.Message = m.loc.T(msgKey, len(files), strings.Join(files, ", "))
		m.screen = ScreenConfirmDelete
	case "r":
		if m.remoteBlocked(active) {
			m.statusMsg = m.loc.T("status_not_connected")
			return m, nil
		}
		entry := active.CurrentFile()
		if entry == nil {
			m.statusMsg = m.loc.T("status_no_files_selected")
			return m, nil
		}
		m.promptMode = "rename"
		m.promptOldName = entry.Name
		m.promptPaneName = m.paneName(active)
		m.promptPane.Reset(entry.Name)
		m.screen = ScreenPrompt
	case "m":
		if m.remoteBlocked(active) {
			m.statusMsg = m.loc.T("status_not_connected")
			return m, nil
		}
		m.promptMode = "mkdir"
		m.promptPaneName = m.paneName(active)
		m.promptPane.Reset("")
		m.screen = ScreenPrompt
	}
	return m, nil
}

// remoteBlocked reports whether pane is the remote pane while no real SSH
// connection exists — d/r/m share this guard rather than each repeating the
// disconnected check inline (same condition enqueueCopyDirection applies to
// transfers).
func (m *Model) remoteBlocked(pane *BrowserPane) bool {
	return pane == m.remotePane && !m.connected && !m.testMode
}

// paneByName returns the BrowserPane matching "local"/"remote", the same
// pane-name convention used by transferDest.
func (m *Model) paneByName(name string) *BrowserPane {
	if name == "local" {
		return m.localPane
	}
	return m.remotePane
}

// paneName is paneByName's inverse: the "local"/"remote" string for a given
// BrowserPane.
func (m *Model) paneName(pane *BrowserPane) string {
	if pane == m.localPane {
		return "local"
	}
	return "remote"
}

// handlePromptKey handles keys in the shared rename/mkdir text-input screen.
func (m *Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenBrowsing
		return m, nil
	case "enter":
		value := m.promptPane.Value()
		if value == "" {
			m.promptPane.ErrMsg = m.loc.T("err_name_required")
			return m, nil
		}
		if strings.Contains(value, "/") {
			m.promptPane.ErrMsg = m.loc.T("err_name_invalid")
			return m, nil
		}
		pane := m.paneByName(m.promptPaneName)
		mode := m.promptMode
		oldName := m.promptOldName
		paneName := m.promptPaneName
		m.screen = ScreenBrowsing
		if mode == "mkdir" {
			return m, mkdirCmd(pane.FS, pane.Cwd, value, paneName)
		}
		if value == oldName {
			// A no-op rename: renameCmd's destination-exists check would
			// otherwise reject this, since Stat(newPath) finds the very
			// file being "renamed" and reports it as a naming conflict.
			return m, nil
		}
		return m, renameCmd(pane.FS, pane.Cwd, oldName, value, paneName)
	}
	cmd := m.promptPane.HandleKey(msg)
	return m, cmd
}

// handleConfirmDeleteKey handles keys in the delete Y/N confirmation screen.
func (m *Model) handleConfirmDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		pane := m.paneByName(m.confirmDeletePaneName)
		targets := m.confirmDeleteTargets
		paneName := m.confirmDeletePaneName
		m.confirmDeleteTargets = nil
		m.confirmDeleteRecursive = false
		m.screen = ScreenBrowsing
		return m, deleteCmd(pane.FS, pane.Cwd, targets, paneName)
	case "n", "N", "esc":
		m.confirmDeleteTargets = nil
		m.confirmDeleteRecursive = false
		m.screen = ScreenBrowsing
	}
	return m, nil
}

// handleAboutKey handles keys in the About overlay.
func (m *Model) handleAboutKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?":
		m.screen = ScreenBrowsing
	}
	return m, nil
}

// handleSettingsKey handles keys in the Settings screen.
func (m *Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Discard: rebuild the theme from the last persisted values.
		m.applyTheme(m.primaryColorHex, m.secondaryColorHex)
		m.screen = ScreenBrowsing
		return m, nil
	case "tab":
		m.settingsPane.NextField()
		return m, nil
	case "shift+tab":
		m.settingsPane.PrevField()
		return m, nil
	case "left":
		if m.settingsPane.Focused() == settingsFieldLingo {
			m.settingsPane.CyclePack(-1)
			return m, nil
		}
	case "right":
		if m.settingsPane.Focused() == settingsFieldLingo {
			m.settingsPane.CyclePack(1)
			return m, nil
		}
	case "enter":
		if err := m.settingsPane.Validate(); err != nil {
			// Error shown inline on the form; stay put.
			return m, nil
		}
		m.loc.SetPack(m.settingsPane.CurrentPack())
		m.primaryColorHex = m.settingsPane.PrimaryValue()
		m.secondaryColorHex = m.settingsPane.SecondaryValue()
		m.applyTheme(m.primaryColorHex, m.secondaryColorHex)

		cfg, err := config.Load()
		if err != nil {
			// A genuine parse failure (not "file missing") means we don't
			// know what's in hosts.yaml — saving here would overwrite it
			// with only the new lingo/theme fields and silently drop the
			// existing Hosts list. Abort instead, matching HostFormPane.Save().
			m.statusMsg = m.loc.T("err_config_load", err)
			m.screen = ScreenBrowsing
			return m, nil
		}
		cfg.Lingo = m.loc.Pack()
		cfg.PrimaryColor = m.primaryColorHex
		cfg.SecondaryColor = m.secondaryColorHex
		if err := cfg.Save(); err != nil {
			m.statusMsg = m.loc.T("err_config_save", err)
		}
		m.screen = ScreenBrowsing
		return m, nil
	}
	// Any other key (character input) goes to whichever color textinput is
	// focused; a no-op if the Lingo Pack row is focused.
	cmd := m.settingsPane.HandleKey(msg)
	return m, cmd
}

// applyTheme rebuilds m.theme from the given hex colors. Both are assumed
// already-valid (either defaults, previously-persisted values, or freshly
// validated by SettingsPane.Validate), so parse errors here are unexpected
// and fall back to the package defaults rather than crash.
func (m *Model) applyTheme(primaryHex, secondaryHex string) {
	primary, err := parseHexColor(primaryHex)
	if err != nil {
		primary = lipgloss.Color(DefaultPrimaryColor)
	}
	secondary, err := parseHexColor(secondaryHex)
	if err != nil {
		secondary = lipgloss.Color(DefaultSecondaryColor)
	}
	m.theme = NewTheme(primary, secondary)
}

// handleHostPickerKey handles keys in the Site Manager overlay.
func (m *Model) handleHostPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.screen = ScreenBrowsing
	case "up":
		m.hostPicker.Up()
	case "down":
		m.hostPicker.Down()
	case "n":
		m.hostForm.ResetForAdd()
		m.screen = ScreenAddHost
	case "e":
		host := m.hostPicker.CurrentHost()
		if host == nil {
			m.statusMsg = m.loc.T("status_no_host_selected")
			return m, nil
		}
		m.hostForm.ResetForEdit(*host)
		m.screen = ScreenAddHost
	case "enter":
		host := m.hostPicker.CurrentHost()
		if host == nil {
			m.statusMsg = m.loc.T("status_no_host_selected")
			return m, nil
		}
		m.screen = ScreenBrowsing
		m.connecting = true
		m.statusMsg = m.loc.T("status_connecting", host.Name)
		return m, tea.Batch(m.connectSSH(*host), m.spinner.Tick)
	}
	return m, nil
}

// handleHostFormKey handles keys in the Add/Edit Host form.
func (m *Model) handleHostFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenHostPicker
		return m, nil
	case "tab":
		m.hostForm.NextField()
		return m, nil
	case "shift+tab":
		m.hostForm.PrevField()
		return m, nil
	case "enter":
		host, err := m.hostForm.Save(m.loc)
		if err != nil {
			// Validation/save error is shown inline on the form; stay put.
			return m, nil
		}
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = m.loc.T("status_hosts_load_error", err)
		}
		m.statusMsg = m.loc.T("status_host_saved", host.Name)
		m.screen = ScreenHostPicker
		return m, nil
	}
	cmd := m.hostForm.HandleKey(msg)
	return m, cmd
}

// connectSSH returns a tea.Cmd that dials the host off the UI thread and
// reports the result as an sshConnectedMsg.
func (m *Model) connectSSH(host config.Host) tea.Cmd {
	return func() tea.Msg {
		if host.Port == 0 {
			host.Port = config.DefaultPort()
		}
		if host.User == "" {
			if u, err := user.Current(); err == nil {
				host.User = u.Username
			}
		}
		sshClient, sftpClient, err := sshclient.Dial(host)
		return sshConnectedMsg{
			host:       host,
			sshClient:  sshClient,
			sftpClient: sftpClient,
			err:        err,
		}
	}
}

// closeSSH tears down the SSH/SFTP connection if one is open.
func (m *Model) closeSSH() {
	if m.sftpClient != nil {
		m.sftpClient.Close()
		m.sftpClient = nil
	}
	if m.sshClient != nil {
		m.sshClient.Close()
		m.sshClient = nil
	}
	m.connected = false
}

// readDirCmd lists a directory on the given filesystem off the UI thread,
// returning the sorted entries as a readDirMsg. Used for the initial remote
// listing after connect, where the ReadDir is a network round-trip.
func readDirCmd(pane string, fs fsys.FileSystem, cwd string) tea.Cmd {
	return func() tea.Msg {
		entries, err := fs.ReadDir(cwd)
		if err != nil {
			return readDirMsg{pane: pane, err: err}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return entries[i].Name < entries[j].Name
		})
		return readDirMsg{pane: pane, entries: entries}
	}
}

func (m *Model) View() string {
	// Budget: pane row + queue pane (fixed 8 rows, up to 5 transfers) +
	// hints line + status line. QueuePane.View() caps its own row count to
	// this height regardless of how many transfers are queued, so this
	// budget holds no matter how many files get selected.
	const queueHeight = 8
	m.localPane.Width = m.width/2 - 2
	m.localPane.Height = m.height - queueHeight - 2

	m.remotePane.Width = m.width/2 - 2
	m.remotePane.Height = m.height - queueHeight - 2

	m.queuePane.Width = m.width - 4
	m.queuePane.Height = queueHeight

	m.aboutPane.Width = m.width - 4
	m.aboutPane.Height = m.height - queueHeight - 2

	m.settingsPane.Width = m.width - 4
	m.settingsPane.Height = m.height - queueHeight - 2

	m.hostPicker.Width = m.width - 4
	m.hostPicker.Height = m.height - queueHeight - 2

	m.hostForm.Width = m.width - 4
	m.hostForm.Height = m.height - queueHeight - 2

	m.promptPane.Width = m.width - 4
	m.promptPane.Height = m.height - queueHeight - 2

	m.confirmPane.Width = m.width - 4
	m.confirmPane.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
		m.remotePane.EmptyMessage = ""
	} else {
		m.remotePane.EmptyMessage = m.loc.T("hint_remote_disconnected")
	}

	localView := m.localPane.View(m.theme)
	remoteView := m.remotePane.View(m.theme)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, localView, remoteView)

	queueView := m.queuePane.View(m.theme, m.loc)

	content := lipgloss.JoinVertical(lipgloss.Left, panes, queueView)

	// The Site Manager is a modal overlay: when active, it replaces the
	// dual-pane content area.
	switch m.screen {
	case ScreenHostPicker:
		content = m.hostPicker.View(m.theme, m.loc)
	case ScreenAddHost:
		content = m.hostForm.View(m.theme, m.loc)
	case ScreenAbout:
		content = m.aboutPane.View(m.theme, m.loc)
	case ScreenSettings:
		previewPrimary, previewSecondary := m.settingsPane.PreviewColors(m.primaryColorHex, m.secondaryColorHex)
		primary, err := parseHexColor(previewPrimary)
		if err != nil {
			primary = lipgloss.Color(DefaultPrimaryColor)
		}
		secondary, err := parseHexColor(previewSecondary)
		if err != nil {
			secondary = lipgloss.Color(DefaultSecondaryColor)
		}
		previewTheme := NewTheme(primary, secondary)
		previewLoc := i18n.NewLocalizer(m.settingsPane.CurrentPack())
		content = m.settingsPane.View(previewTheme, previewLoc)
	case ScreenPrompt:
		headerKey := "rename_header"
		if m.promptMode == "mkdir" {
			headerKey = "mkdir_header"
		}
		content = m.promptPane.View(m.theme, m.loc, headerKey)
	case ScreenConfirmDelete:
		confirmHeaderKey := "confirm_delete_header"
		if m.confirmDeleteRecursive {
			confirmHeaderKey = "confirm_delete_recursive_header"
		}
		content = m.confirmPane.View(m.theme, m.loc, confirmHeaderKey)
	}

	status := m.statusMsg
	if m.connecting {
		status = m.spinner.View() + " " + status
	}
	statusBar := m.theme.StatusBar.Render(status)

	footer := content

	// The browsing screen's key hints are shown on their own persistent line
	// so a transient status message (e.g. "Connected to host") never hides
	// them. The host picker/form screens embed their own hints in their
	// title, so only the transient status line applies there.
	if m.screen == ScreenBrowsing {
		hintsBar := m.theme.StatusBar.Render(m.loc.T("hint_bar"))
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, hintsBar)
	}

	footer = lipgloss.JoinVertical(lipgloss.Left, footer, statusBar)

	return footer
}

// enqueueCopy copies from the currently focused pane to the other pane.
func (m *Model) enqueueCopy() tea.Cmd {
	if m.localPane.Focus {
		return m.enqueueCopyDirection(m.localPane, m.remotePane)
	}
	return m.enqueueCopyDirection(m.remotePane, m.localPane)
}

// enqueueCopyDirection copies from srcPane to dstPane regardless of focus.
// Used by the Left/Right arrow shortcuts, which point in the direction the
// file travels: Right pushes local → remote, Left pulls remote → local.
//
// The guard checks and the marked-entry snapshot below run synchronously —
// as part of handling the keypress on Update()'s own goroutine, before the
// tea.Cmd is even constructed, not inside it. BrowserPane's Cwd/Entries/FS
// fields have no synchronization (Enter()/Back() mutate them directly from
// Update()), and the returned Cmd's walk can now take long enough (real
// directory I/O, possibly over SFTP) that reading those fields
// progressively during the walk — as the pre-#6 code did, back when the
// whole operation was one near-instant loop — would race against the user
// navigating either pane mid-walk. Snapshotting here means the returned
// Cmd only ever touches local values, m.eventsCh/m.jobsCh, and the
// mutex-guarded allocateTransferID, matching the same discipline
// enqueueFileCopy/enqueueDirectoryCopy (copyops.go) already follow.
func (m *Model) enqueueCopyDirection(srcPane, dstPane *BrowserPane) tea.Cmd {
	if (srcPane == m.remotePane || dstPane == m.remotePane) && !m.connected && !m.testMode {
		m.statusMsg = m.loc.T("status_not_connected")
		return nil
	}

	files := srcPane.GetSelectedFiles()
	if len(files) == 0 {
		if entry := srcPane.CurrentFile(); entry != nil {
			files = []string{entry.Name}
		}
	}
	if len(files) == 0 {
		m.statusMsg = m.loc.T("status_no_files_selected")
		return nil
	}

	srcName := m.paneName(srcPane)
	dstName := m.paneName(dstPane)

	type copyEntry struct {
		name  string
		isDir bool
		size  int64
		group *dirCopyGroup
	}
	entries := make([]copyEntry, len(files))
	for i, filename := range files {
		ce := copyEntry{name: filename}
		if e := srcPane.EntryByName(filename); e != nil {
			ce.isDir = e.IsDir
			ce.size = e.Size
		}
		if ce.isDir {
			// Registered now, synchronously, rather than from the walk
			// goroutine below — see newDirCopyGroup.
			ce.group = newDirCopyGroup(srcPane, ce.name)
		}
		entries[i] = ce
	}

	srcFS, srcCwd := srcPane.FS, srcPane.Cwd
	dstFS, dstCwd := dstPane.FS, dstPane.Cwd

	return func() tea.Msg {
		totalFiles := 0
		for _, ce := range entries {
			srcPath := srcFS.Join(srcCwd, ce.name)
			dstPath := dstFS.Join(dstCwd, ce.name)

			if ce.isDir {
				count := m.enqueueDirectoryCopy(srcFS, dstFS, srcPath, dstPath, ce.name, ce.group, srcName, dstName)
				// Sent only after the recursive walk for this top-level
				// directory has fully returned, so it always arrives after
				// every transferQueuedMsg/transferQueueErrorMsg the walk
				// produced for this group (same goroutine, same channel,
				// FIFO order) — see maybeFinalizeGroup.
				m.eventsCh <- dirWalkDoneMsg{group: ce.group, discovered: count}
				totalFiles += count
				continue
			}
			m.enqueueFileCopy(srcFS, dstFS, srcPath, dstPath, ce.name, ce.size, nil, srcName, dstName)
			totalFiles++
		}

		if totalFiles > 0 {
			// The usual per-file TransferDoneMsg refresh (unchanged, see
			// that case above) will show the destination pane's current
			// state once any file completes — including any empty sibling
			// directories already created by MkdirAll above. Deliberately
			// NOT also refreshing here: this Cmd and each worker's
			// TransferDoneMsg-triggered readDirCmd would otherwise be two
			// independent, unordered refreshes of the same pane, and a
			// slower one arriving after a faster one would silently
			// overwrite a fresher listing with stale data.
			return nil
		}

		// Nothing was enqueued (e.g. every marked entry was an empty
		// directory, or every directory failed to walk) — nothing will
		// ever trigger the per-file refresh above, so do it explicitly. A
		// direct synchronous call, not a returned tea.Cmd for bubbletea to
		// schedule separately: it must run strictly after the walk above,
		// in the same goroutine — and since totalFiles == 0 here, there's
		// no competing TransferDoneMsg-triggered refresh for it to race.
		return readDirCmd(dstName, dstFS, dstCwd)()
	}
}

// transferInfo records a queued transfer's source and destination panes
// (by "local"/"remote" name) and the filename involved, keyed by transfer
// ID in Model.transferDest. group is non-nil when this file came from a
// recursive directory copy — see dirCopyGroup.
type transferInfo struct {
	destPane string
	srcPane  string
	filename string
	group    *dirCopyGroup
}

// setTransferDest and popTransferDest guard m.transferDest with a mutex.
// Both are only ever called from Update()'s own goroutine today (see
// transferDestMu's comment on Model), so the mutex is defense-in-depth
// rather than a live requirement — a plain map write racing a map
// read/delete is a fatal Go runtime error, not just a benign data race, so
// it's cheap insurance against a future direct-mutation call site making
// that mistake again.
func (m *Model) setTransferDest(id int, destName, srcName, filename string, group *dirCopyGroup) {
	m.transferDestMu.Lock()
	defer m.transferDestMu.Unlock()
	m.transferDest[id] = transferInfo{destPane: destName, srcPane: srcName, filename: filename, group: group}
}

func (m *Model) popTransferDest(id int) (transferInfo, bool) {
	m.transferDestMu.Lock()
	defer m.transferDestMu.Unlock()
	info, ok := m.transferDest[id]
	delete(m.transferDest, id)
	return info, ok
}

// allocateTransferID hands out a unique transfer ID, guarded by nextIDMu.
// Safe to call from any goroutine — see nextIDMu's comment on Model.
func (m *Model) allocateTransferID() int {
	m.nextIDMu.Lock()
	defer m.nextIDMu.Unlock()
	id := m.nextID
	m.nextID++
	return id
}

// dirCopyGroup tracks one marked directory's recursive-copy operation, so
// its own selection mark (the directory itself, not any file inside it) can
// be cleared once every file the walk discovered has finished — but only if
// every one of them succeeded. A single failure anywhere in the subtree
// leaves the mark in place, on the same logic as a single failed flat-file
// copy: the transfer queue's error row already shows what failed, and the
// directory mark being left on is the signal that this directory still
// needs another push.
//
// discovered isn't known until the walk itself finishes (walkDone), since
// enqueueDirectoryCopy discovers files lazily while recursing — completed
// is compared against discovered only once walkDone is true, so a group
// with zero files completed so far isn't mistaken for "already finished"
// while its walk is still in progress.
//
// A *dirCopyGroup is created once (newDirCopyGroup) and then only ever
// created, read, or mutated from Update()'s own goroutine — via
// transferQueuedMsg/dirWalkDoneMsg/transferQueueErrorMsg (copyops.go's walk
// goroutine only ever carries a *dirCopyGroup along on a message, never
// touches its fields directly) and TransferDoneMsg/TransferErrorMsg (via
// completeGroupFile). So, like transferDest's underlying values, no mutex
// is needed. There's deliberately no map keyed by an allocated ID (unlike
// Model.transferDest): nothing ever needs to look a group up "from
// outside" — every read site already has the *dirCopyGroup in hand via the
// message or transferInfo that's carrying it.
type dirCopyGroup struct {
	srcPane    *BrowserPane
	dirName    string
	walkDone   bool
	discovered int
	completed  int
	failed     bool
}

// newDirCopyGroup registers a new directory-copy operation for dirName in
// srcPane and returns it. Only ever called from enqueueCopyDirection's
// synchronous guard/snapshot phase (Update()'s own goroutine, before the
// walk's tea.Cmd is even returned) — see dirCopyGroup's comment for why
// that makes a mutex unnecessary here, unlike allocateTransferID.
//
// Recorded on srcPane.activeCopyGroups[dirName], overwriting any earlier
// group for the same name — see that field's comment on BrowserPane for
// why: it lets maybeFinalizeGroup detect a stale, already-superseded group
// (the user re-marked and re-pushed the same directory before an earlier
// push of it finished) and skip clearing a mark that belongs to the newer
// push instead.
func newDirCopyGroup(srcPane *BrowserPane, dirName string) *dirCopyGroup {
	g := &dirCopyGroup{srcPane: srcPane, dirName: dirName}
	srcPane.activeCopyGroups[dirName] = g
	return g
}

// completeGroupFile records one file's completion (success or failure)
// against its directory-copy group, if it belongs to one (a nil group
// means a flat, non-directory copy — a no-op here). Called from
// TransferDoneMsg/TransferErrorMsg's handlers in Update().
func completeGroupFile(g *dirCopyGroup, failed bool) {
	if g == nil {
		return
	}
	g.completed++
	if failed {
		g.failed = true
	}
	maybeFinalizeGroup(g)
}

// maybeFinalizeGroup clears g's directory mark once its walk has finished
// discovering files AND every discovered file has completed — but only
// clears the mark if none of them failed. Safe to call redundantly (e.g.
// once from a file completion and once from dirWalkDoneMsg): the
// activeCopyGroups check below means only the call that actually observes
// both conditions true, for the still-current group, does anything.
//
// If g is no longer srcPane.activeCopyGroups[dirName] (a newer push of the
// same name has since replaced it — see newDirCopyGroup), this is a stale
// finish and must NOT touch the current mark, which belongs to that newer
// group.
func maybeFinalizeGroup(g *dirCopyGroup) {
	if !g.walkDone || g.completed < g.discovered {
		return
	}
	if g.srcPane.activeCopyGroups[g.dirName] != g {
		return
	}
	delete(g.srcPane.activeCopyGroups, g.dirName)
	if !g.failed {
		g.srcPane.ClearSelected(g.dirName)
	}
}

// waitForEvent is the "subscription" pattern in bubbletea.
// It returns a tea.Cmd that blocks on receiving one message from ch,
// then delivers it to Update. It must be re-armed after each message
// (i.e., return this command again) or the subscription dies.
// Workers send progress/done/error messages on this channel continuously.
func waitForEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
