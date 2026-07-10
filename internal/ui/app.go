package ui

import (
	"fmt"
	"log"
	"os"

	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/bvanhorn/exfil/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Screen string

const (
	ScreenBrowsing   Screen = "browsing"
	ScreenHostPicker Screen = "hostpicker"
	ScreenAddHost    Screen = "addhost"
)

// Messages
type readDirMsg struct {
	pane    string
	entries []fsys.Entry
	err     error
}

type sshConnectedMsg struct {
	sftpClient interface{}
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
	jobsCh      chan transfer.Job
	logger      *log.Logger
}

func NewModel(eventsCh chan tea.Msg, jobsCh chan transfer.Job, logger *log.Logger) *Model {
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
		jobsCh:    jobsCh,
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
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "tab":
			m.localPane.SetFocus(!m.localPane.Focus)
			m.remotePane.SetFocus(m.localPane.Focus)
		case "up":
			if m.localPane.Focus {
				m.localPane.Up()
			} else {
				m.remotePane.Up()
			}
		case "down":
			if m.localPane.Focus {
				m.localPane.Down()
			} else {
				m.remotePane.Down()
			}
		case "enter":
			if m.localPane.Focus {
				if err := m.localPane.Enter(); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
				}
			} else {
				if err := m.remotePane.Enter(); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
				}
			}
		case "backspace":
			if m.localPane.Focus {
				if err := m.localPane.Back(); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
				}
			} else {
				if err := m.remotePane.Back(); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
				}
			}
		case " ":
			if m.localPane.Focus {
				m.localPane.ToggleSelect()
			} else {
				m.remotePane.ToggleSelect()
			}
		case "c":
			return m, m.enqueueCopy()
		}

	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error reading dir: %v", msg.err)
		}
		if msg.pane == "local" {
			m.localPane.Entries = msg.entries
		} else if msg.pane == "remote" {
			m.remotePane.Entries = msg.entries
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

func (m *Model) enqueueCopy() tea.Cmd {
	return func() tea.Msg {
		var srcPane, dstPane *BrowserPane
		if m.localPane.Focus {
			srcPane = m.localPane
			dstPane = m.remotePane
		} else {
			srcPane = m.remotePane
			dstPane = m.localPane
		}

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
