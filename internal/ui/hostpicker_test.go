package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func TestHostPickerPaneViewListsHosts(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Hosts = []config.Host{
		{Name: "wintermute", Hostname: "192.168.1.51", User: "daddy"},
	}
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "wintermute") {
		t.Errorf("expected the host name in the view, got:\n%s", view)
	}
}

func TestHostPickerPaneViewEmptyStillHasBorder(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box even with no hosts, got:\n%s", view)
	}
}

// TestHostPickerPaneViewBorderUsesThemeGradient is a regression test for
// the visual-effects feature: Host Picker previously rendered as plain
// unbordered text — it must now show an actual color gradient, not a flat
// border.
func TestHostPickerPaneViewBorderUsesThemeGradient(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Hosts = []config.Host{{Name: "wintermute", Hostname: "192.168.1.51", User: "daddy"}}
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
