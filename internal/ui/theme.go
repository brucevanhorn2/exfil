package ui

import (
	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	// Pane borders
	PaneBorder       lipgloss.Style
	PaneBorderFocus  lipgloss.Style
	PaneTitle        lipgloss.Style
	PaneTitleFocus   lipgloss.Style

	// Browser content
	BrowserDir       lipgloss.Style
	BrowserFile      lipgloss.Style
	BrowserSelected  lipgloss.Style
	BrowserCursor    lipgloss.Style

	// Queue pane
	QueueBorder      lipgloss.Style
	QueueTitle       lipgloss.Style
	TransferQueued   lipgloss.Style
	TransferRunning  lipgloss.Style
	TransferDone     lipgloss.Style
	TransferError    lipgloss.Style
	ProgressBar      lipgloss.Style

	// Status bar
	StatusBar        lipgloss.Style
	StatusKey        lipgloss.Style
	StatusValue      lipgloss.Style
	StatusError      lipgloss.Style
}

func NewTheme() Theme {
	return Theme{
		// Pane borders
		PaneBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Foreground(lipgloss.Color("7")).
			Padding(0, 1),

		PaneBorderFocus: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("5")).
			Foreground(lipgloss.Color("7")).
			Padding(0, 1),

		PaneTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true),

		PaneTitleFocus: lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Bold(true),

		// Browser items
		BrowserDir: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		BrowserFile: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		BrowserSelected: lipgloss.NewStyle().
			Background(lipgloss.Color("5")).
			Foreground(lipgloss.Color("0")),

		BrowserCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		// Queue
		QueueBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1),

		QueueTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true),

		TransferQueued: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),

		TransferRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")),

		TransferDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")),

		TransferError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),

		ProgressBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		StatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		StatusValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")),

		StatusError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),
	}
}
