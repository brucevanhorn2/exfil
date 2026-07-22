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
	nextID            int

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
	// Written from enqueueCopyDirection's tea.Cmd goroutine and read/deleted
	// from Update()'s goroutine, so access must go through transferDestMu —
	// a plain map write racing a map read/delete is a fatal Go runtime
	// error, not just a benign data race.
	transferDest   map[int]transferInfo
	transferDestMu sync.Mutex
	eventsCh       chan tea.Msg
	jobsCh         chan transfer.Job
	logger         *log.Logger

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
		// files (still in flight or yet to complete) are untouched.
		m.paneByName(info.srcPane).ClearSelected(info.filename)
		dstPane := m.paneByName(info.destPane)
		// Re-list whichever directory the destination pane currently shows,
		// so the newly-arrived file appears without the user navigating
		// away and back.
		return m, tea.Batch(waitForEvent(m.eventsCh), readDirCmd(info.destPane, dstPane.FS, dstPane.Cwd))

	case transfer.TransferErrorMsg:
		// Transfer failed. Mark it as error and keep the error message visible.
		m.queuePane.UpdateTransfer(msg.ID, StatusError, 0, 0, "", msg.Err.Error())
		m.popTransferDest(msg.ID)
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
func (m *Model) enqueueCopyDirection(srcPane, dstPane *BrowserPane) tea.Cmd {
	return func() tea.Msg {
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

		dstName := m.paneName(dstPane)
		srcName := m.paneName(srcPane)

		for _, filename := range files {
			entry := srcPane.EntryByName(filename)
			if entry != nil && entry.IsDir {
				m.statusMsg = m.loc.T("status_dir_not_supported")
				continue
			}

			id := m.nextID
			m.nextID++
			m.setTransferDest(id, dstName, srcName, filename)

			srcPath := srcPane.FS.Join(srcPane.Cwd, filename)
			dstPath := dstPane.FS.Join(dstPane.Cwd, filename)

			var size int64
			if entry != nil {
				size = entry.Size
			}

			m.queuePane.AddTransfer(Transfer{
				ID:       id,
				Filename: filename,
				Status:   StatusQueued,
				Total:    size,
			})

			job := transfer.Job{
				ID:         id,
				SourcePath: srcPath,
				DestPath:   dstPath,
				Filename:   filename,
				// Carry each side's filesystem so the worker knows whether this
				// is a local copy, a download (remote→local), or an upload.
				SrcFS: srcPane.FS,
				DstFS: dstPane.FS,
			}

			m.jobsCh <- job
		}

		return nil
	}
}

// transferInfo records a queued transfer's source and destination panes
// (by "local"/"remote" name) and the filename involved, keyed by transfer
// ID in Model.transferDest.
type transferInfo struct {
	destPane string
	srcPane  string
	filename string
}

// setTransferDest and popTransferDest guard m.transferDest with a mutex:
// setTransferDest is called from enqueueCopyDirection's tea.Cmd goroutine,
// popTransferDest from Update()'s goroutine, and a plain map has no built-in
// protection against that — Go's runtime treats a concurrent map
// write/read as a fatal, unrecoverable error rather than a benign race.
func (m *Model) setTransferDest(id int, destName, srcName, filename string) {
	m.transferDestMu.Lock()
	defer m.transferDestMu.Unlock()
	m.transferDest[id] = transferInfo{destPane: destName, srcPane: srcName, filename: filename}
}

func (m *Model) popTransferDest(id int) (transferInfo, bool) {
	m.transferDestMu.Lock()
	defer m.transferDestMu.Unlock()
	info, ok := m.transferDest[id]
	delete(m.transferDest, id)
	return info, ok
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
