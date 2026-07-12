package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/version"
	"github.com/charmbracelet/lipgloss"
)

// logo is a "pagga"-style ASCII rendering of "exfil" (via `toilet -f pagga`),
// colored at render time to match the app's cyberpunk theme instead of
// baking in ANSI codes here.
const logo = `░█▀▀░█░█░█▀▀░▀█▀░█░░
░█▀▀░▄▀▄░█▀▀░░█░░█░░
░▀▀▀░▀░▀░▀░░░▀▀▀░▀▀▀`

type AboutPane struct {
	theme  Theme
	Width  int
	Height int
}

func NewAboutPane(theme Theme) *AboutPane {
	return &AboutPane{theme: theme}
}

func (a *AboutPane) View() string {
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)

	lines := []string{
		logoStyle.Render(logo),
		"",
		a.theme.BrowserFile.Render("cyberpunk TUI SCP/SFTP client"),
		"",
		a.theme.BrowserDir.Render("Version:  ") + version.Version,
		a.theme.BrowserDir.Render("License:  ") + "MIT",
		a.theme.BrowserDir.Render("Source:   ") + "github.com/brucevanhorn2/exfil",
		"",
		a.theme.PaneTitle.Render("[Esc/q] close"),
	}

	content := strings.Join(lines, "\n")
	return a.theme.PaneBorderFocus.Width(a.Width).Height(a.Height).Render(content)
}
