package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParseHexColorValid(t *testing.T) {
	c, err := parseHexColor("#B341F5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != lipgloss.Color("#B341F5") {
		t.Errorf("got %v, want #B341F5", c)
	}
}

func TestParseHexColorInvalid(t *testing.T) {
	tests := []string{"", "B341F5", "#B341F", "#GGGGGG", "purple"}
	for _, in := range tests {
		if _, err := parseHexColor(in); err == nil {
			t.Errorf("parseHexColor(%q): expected error, got none", in)
		}
	}
}

func TestNewThemeAppliesPrimaryAndSecondary(t *testing.T) {
	primary := lipgloss.Color("#39FF14")
	secondary := lipgloss.Color("#3A3A4A")
	theme := NewTheme(primary, secondary)

	if theme.PaneTitleFocus.GetForeground() != primary {
		t.Errorf("PaneTitleFocus foreground = %v, want %v", theme.PaneTitleFocus.GetForeground(), primary)
	}
	if theme.PaneTitle.GetForeground() != secondary {
		t.Errorf("PaneTitle foreground = %v, want %v", theme.PaneTitle.GetForeground(), secondary)
	}
	if theme.BrowserSelected.GetBackground() != primary {
		t.Errorf("BrowserSelected background = %v, want %v", theme.BrowserSelected.GetBackground(), primary)
	}
	if theme.QueueTitle.GetForeground() != secondary {
		t.Errorf("QueueTitle foreground = %v, want %v", theme.QueueTitle.GetForeground(), secondary)
	}
}

func TestNewThemeStoresRawGradientColors(t *testing.T) {
	primary := lipgloss.Color("#39FF14")
	secondary := lipgloss.Color("#3A3A4A")
	theme := NewTheme(primary, secondary)

	if theme.PrimaryColor != primary {
		t.Errorf("PrimaryColor = %v, want %v", theme.PrimaryColor, primary)
	}
	if theme.SecondaryColor != secondary {
		t.Errorf("SecondaryColor = %v, want %v", theme.SecondaryColor, secondary)
	}
	if theme.MutedPrimaryColor != mutedColor(primary) {
		t.Errorf("MutedPrimaryColor = %v, want %v", theme.MutedPrimaryColor, mutedColor(primary))
	}
	if theme.MutedSecondaryColor != mutedColor(secondary) {
		t.Errorf("MutedSecondaryColor = %v, want %v", theme.MutedSecondaryColor, mutedColor(secondary))
	}
}
