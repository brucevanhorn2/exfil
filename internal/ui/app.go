package ui

import (
	"fmt"
	"log"
	"os"

	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Screen string

const (
	ScreenBrowsing Screen = "browsing"
	ScreenHostPicker Screen = "hostpicker"
	ScreenAddHost    Screen = "addhost"
)

// Messages
type readDirMsg struct {
	pane  string // "local" or "remote"
	entries []fsys.Entry
	err   error
}

type sshConnectedMsg struct {
	err error
}

type transferStartedMsg struct {
	ID       int
	Filename string
	Total    int64
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

type winSizeMsg struct {
	width  int
	height int
}

// Model is the root bubbletea model
type Model struct {
	width       int
	height      int
	screen      Screen
	theme       Theme
	localPane   *BrowserPane
	remotePane  *BrowserPane
	queuePane   *QueuePane
	statusMsg   string
	nextID      int
	eventsCh    chan tea.Msg
	logger      *log.Logger
}

func NewModel(eventsCh chan tea.Msg, logger *log.Logger) *Model {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	theme := NewTheme()
	localFS := fsys.LocalFS{}
	home, _ := localFS.Home()

	m := &Model{
		screen:    ScreenBrowsing,
		theme:     theme,
		eventsCh:  eventsCh,
		logger:    logger,
		localPane: NewBrowserPane("local", localFS, theme),
		remotePane: NewBrowserPane("remote", fsys.LocalFS{}, theme),
		queuePane: NewQueuePane(theme),
		statusMsg: "Ready. [Tab] switch pane  [↑/↓] navigate  [↵] enter  [⌫] back  [space] select  [c] copy  [q] quit",
		nextID:    1,
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
		waitForEvent(m.eventsCh),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.localPane.Width = m.width/2 - 2
		m.localPane.Height = m.height - 6
		m.remotePane.Width = m.width/2 - 2
		m.remotePane.Height = m.height - 6
		m.queuePane.Width = m.width - 4
		m.queuePane.Height = 3

	case tea.KeyMsg:
		keyMsg := handleKey(msg)
		return m.handleKey(keyMsg)

	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error reading dir: %v", msg.err)
		}
		if msg.pane == "local" {
			m.localPane.Entries = msg.entries
		} else if msg.pane == "remote" {
			m.remotePane.Entries = msg.entries
		}

	case transferProgressMsg:
		m.queuePane.UpdateTransfer(msg.ID, StatusRunning, msg.Done, msg.Total, msg.Speed, "")
		return m, waitForEvent(m.eventsCh)

	case transferDoneMsg:
		m.queuePane.UpdateTransfer(msg.ID, StatusDone, 0, 0, "", "")
		return m, waitForEvent(m.eventsCh)

	case transferErrorMsg:
		m.queuePane.UpdateTransfer(msg.ID, StatusError, 0, 0, "", msg.Err.Error())
		return m, waitForEvent(m.eventsCh)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg {
	case KeyQuit:
		return m, tea.Quit
	case KeyTab:
		m.localPane.SetFocus(!m.localPane.Focus)
		m.remotePane.SetFocus(m.localPane.Focus)
	case KeyUp:
		if m.localPane.Focus {
			m.localPane.Up()
		} else {
			m.remotePane.Up()
		}
	case KeyDown:
		if m.localPane.Focus {
			m.localPane.Down()
		} else {
			m.remotePane.Down()
		}
	case KeyEnter:
		if m.localPane.Focus {
			if err := m.localPane.Enter(); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
			}
		} else {
			if err := m.remotePane.Enter(); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
			}
		}
	case KeyBackspace:
		if m.localPane.Focus {
			if err := m.localPane.Back(); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
			}
		} else {
			if err := m.remotePane.Back(); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
			}
		}
	case KeySpace:
		if m.localPane.Focus {
			m.localPane.ToggleSelect()
		} else {
			m.remotePane.ToggleSelect()
		}
	}

	return m, nil
}

func (m *Model) View() string {
	m.localPane.Width = m.width/2 - 3
	m.localPane.Height = m.height - 8

	m.remotePane.Width = m.width/2 - 3
	m.remotePane.Height = m.height - 8

	m.queuePane.Width = m.width - 4
	m.queuePane.Height = 4

	localView := m.localPane.View()
	remoteView := m.remotePane.View()

	panes := lipgloss.JoinHorizontal(lipgloss.Top, localView, remoteView)

	queueView := m.queuePane.View()

	content := lipgloss.JoinVertical(lipgloss.Left, panes, queueView)

	statusBar := m.theme.StatusBar.Render(m.statusMsg)

	footer := lipgloss.JoinVertical(lipgloss.Left, content, statusBar)

	return footer
}

func waitForEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
