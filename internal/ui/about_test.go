package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func TestAboutPaneViewIncludesTaglineAndCloseHint(t *testing.T) {
	a := NewAboutPane()
	a.Width = 60
	a.Height = 20

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	loc := i18n.NewLocalizer("plain")
	view := a.View(theme, loc)

	if !strings.Contains(view, "cyberpunk TUI SCP/SFTP client") {
		t.Errorf("expected the plain-pack tagline in the view, got:\n%s", view)
	}
	if !strings.Contains(view, "[Esc/q] close") {
		t.Errorf("expected the close hint in the view, got:\n%s", view)
	}
}

// TestAboutPaneViewBorderUsesThemeGradient is a regression test for the
// visual-effects feature: the border must actually vary in color between
// its primary-colored and secondary-colored endpoints, not stay one flat
// color.
func TestAboutPaneViewBorderUsesThemeGradient(t *testing.T) {
	a := NewAboutPane()
	a.Width = 60
	a.Height = 20

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	loc := i18n.NewLocalizer("plain")
	view := a.View(theme, loc)

	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;0;0;255") {
		t.Errorf("expected the bottom-right corner to be pure blue, got:\n%s", view)
	}
}
