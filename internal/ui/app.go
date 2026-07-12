package ui

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"sort"

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
	ScreenBrowsing   Screen = "browsing"
	ScreenHostPicker Screen = "hostpicker"
	ScreenAddHost    Screen = "addhost"
	ScreenAbout      Screen = "about"
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

type transferProgressMsg struct {
	ID    int
	Done  int64
	Total int64
	Speed string
}

type transferDoneMsg struct {
	ID int
}

type transferErrorMsg struct {
	ID  int
	Err error
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
	queuePane         *QueuePane
	statusMsg         string
	nextID            int
	eventsCh          chan tea.Msg
	jobsCh            chan transfer.Job
	logger            *log.Logger

	// SSH connection state. Held so we can close cleanly and so the remote
	// pane's RemoteFS shares the single sftp client (safe for concurrent use).
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	connected  bool

	// connecting is true while an SSH dial is in flight; drives the spinner.
	spinner    spinner.Model
	connecting bool
}

func NewModel(eventsCh chan tea.Msg, jobsCh chan transfer.Job, logger *log.Logger) *Model {
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
	hostForm := NewHostFormPane(theme)
	aboutPane := NewAboutPane(theme)

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
		queuePane:         NewQueuePane(),
		spinner:           sp,
		statusMsg:         "Ready.",
		nextID:            1,
	}

	m.localPane.Cwd = home
	m.localPane.SetFocus(true)
	m.remotePane.SetFocus(false)

	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			if err := m.localPane.Refresh(); err != nil {
				return readDirMsg{pane: "local", err: err}
			}
			return readDirMsg{pane: "local", entries: m.localPane.Entries}
		},
		func() tea.Msg {
			// Refresh the remote pane too, even before an SSH connection is
			// made: it defaults to a LocalFS rooted at "/", which lets both
			// panes be used for local-to-local testing (see README). Once
			// connected, sshConnectedMsg's readDirCmd overwrites this with
			// the real remote listing.
			if err := m.remotePane.Refresh(); err != nil {
				return readDirMsg{pane: "remote", err: err}
			}
			return readDirMsg{pane: "remote", entries: m.remotePane.Entries}
		},
		waitForEvent(m.eventsCh),
	)
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
			m.statusMsg = fmt.Sprintf("Connection to %s failed: %v", msg.host.Name, msg.err)
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
		m.statusMsg = fmt.Sprintf("Connected to %s@%s", msg.host.User, msg.host.Hostname)
		// List the remote directory off the UI thread (network call).
		return m, readDirCmd("remote", rfs, cwd)

	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error reading dir: %v", msg.err)
		}
		if msg.pane == "local" {
			m.localPane.SetEntries(msg.entries)
		} else if msg.pane == "remote" {
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
		// TODO (M4): Refresh destination pane listing to show the new file
		return m, waitForEvent(m.eventsCh)

	case transfer.TransferErrorMsg:
		// Transfer failed. Mark it as error and keep the error message visible.
		m.queuePane.UpdateTransfer(msg.ID, StatusError, 0, 0, "", msg.Err.Error())
		return m, waitForEvent(m.eventsCh)
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
			m.statusMsg = fmt.Sprintf("Error loading hosts: %v", err)
		}
		m.screen = ScreenHostPicker
	case "?":
		m.screen = ScreenAbout
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
			m.statusMsg = fmt.Sprintf("Error: %v", err)
		}
	case "backspace":
		if err := active.Back(); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
		}
	case " ":
		active.ToggleSelect()
	case "c":
		return m, m.enqueueCopy()
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
			m.statusMsg = "No host selected"
			return m, nil
		}
		m.hostForm.ResetForEdit(*host)
		m.screen = ScreenAddHost
	case "enter":
		host := m.hostPicker.CurrentHost()
		if host == nil {
			m.statusMsg = "No host selected"
			return m, nil
		}
		m.screen = ScreenBrowsing
		m.connecting = true
		m.statusMsg = fmt.Sprintf("Connecting to %s…", host.Name)
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
		host, err := m.hostForm.Save()
		if err != nil {
			// Validation/save error is shown inline on the form; stay put.
			return m, nil
		}
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = fmt.Sprintf("Error reloading hosts: %v", err)
		}
		m.statusMsg = fmt.Sprintf("Saved host %q", host.Name)
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

	localView := m.localPane.View(m.theme)
	remoteView := m.remotePane.View(m.theme)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, localView, remoteView)

	queueView := m.queuePane.View(m.theme, m.loc)

	content := lipgloss.JoinVertical(lipgloss.Left, panes, queueView)

	// The Site Manager is a modal overlay: when active, it replaces the
	// dual-pane content area.
	if m.screen == ScreenHostPicker {
		content = m.hostPicker.View(m.theme, m.loc)
	} else if m.screen == ScreenAddHost {
		content = m.hostForm.View()
	} else if m.screen == ScreenAbout {
		content = m.aboutPane.View()
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
		hintsBar := m.theme.StatusBar.Render(
			"[Tab] switch pane  [↑/↓] nav  [→] push→remote  [←] pull←local  [↵] enter  [⌫] back  [space] select  [s] hosts  [?] about  [q] quit",
		)
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
		files := srcPane.GetSelectedFiles()
		if len(files) == 0 {
			if entry := srcPane.CurrentFile(); entry != nil {
				files = []string{entry.Name}
			}
		}

		if len(files) == 0 {
			m.statusMsg = "No files selected"
			return nil
		}

		for _, filename := range files {
			entry := &fsys.Entry{}
			for _, e := range srcPane.Entries {
				if e.Name == filename {
					entry = &e
					break
				}
			}

			if entry.IsDir {
				m.statusMsg = "Directories not supported"
				continue
			}

			id := m.nextID
			m.nextID++

			srcPath := srcPane.FS.Join(srcPane.Cwd, filename)
			dstPath := dstPane.FS.Join(dstPane.Cwd, filename)

			m.queuePane.AddTransfer(Transfer{
				ID:       id,
				Filename: filename,
				Status:   StatusQueued,
				Total:    entry.Size,
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
