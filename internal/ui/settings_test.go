package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsPaneResetFromConfigFindsPackIndex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("keyboardcowboy", "#39FF14", "#3A3A4A")
	if s.CurrentPack() != "keyboardcowboy" {
		t.Errorf("CurrentPack() = %q, want %q", s.CurrentPack(), "keyboardcowboy")
	}
	if s.PrimaryValue() != "#39FF14" {
		t.Errorf("PrimaryValue() = %q, want %q", s.PrimaryValue(), "#39FF14")
	}
}

func TestSettingsPaneResetFromConfigUnknownPackDefaultsToFirst(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("nonexistent", "#39FF14", "#3A3A4A")
	if s.CurrentPack() != i18n.Packs()[0] {
		t.Errorf("expected fallback to first pack, got %q", s.CurrentPack())
	}
}

func TestSettingsPaneCyclePackWrapsAround(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")

	packs := i18n.Packs()
	s.CyclePack(-1) // from index 0, should wrap to the last pack
	if s.CurrentPack() != packs[len(packs)-1] {
		t.Errorf("CyclePack(-1) from first pack = %q, want %q (wrap to last)", s.CurrentPack(), packs[len(packs)-1])
	}

	s.CyclePack(1) // back to first
	if s.CurrentPack() != packs[0] {
		t.Errorf("CyclePack(1) = %q, want %q", s.CurrentPack(), packs[0])
	}
}

func TestSettingsPaneFieldNavigationWraps(t *testing.T) {
	s := NewSettingsPane()
	if s.Focused() != settingsFieldLingo {
		t.Fatalf("expected initial focus on Lingo row, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldPrimary {
		t.Errorf("expected focus on Primary after one NextField, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldSecondary {
		t.Errorf("expected focus on Secondary after two NextField, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldLingo {
		t.Errorf("expected wrap back to Lingo after three NextField, got %v", s.Focused())
	}
	s.PrevField()
	if s.Focused() != settingsFieldSecondary {
		t.Errorf("expected wrap to Secondary after PrevField from Lingo, got %v", s.Focused())
	}
}

func TestSettingsPaneValidateRejectsInvalidHex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "not-a-color", "#6E6E6E")
	if err := s.Validate(); err == nil {
		t.Error("expected Validate() to reject an invalid primary color, got nil")
	}
}

func TestSettingsPaneValidateAcceptsValidHex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	if err := s.Validate(); err != nil {
		t.Errorf("expected Validate() to accept valid hex colors, got: %v", err)
	}
}

func TestSettingsPanePreviewColorsFallsBackOnIncompleteInput(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	s.primaryInput.SetValue("#B3") // incomplete, mid-typing

	primary, secondary := s.PreviewColors("#000000", "#111111")
	if primary != "#000000" {
		t.Errorf("expected incomplete input to hold fallback %q, got %q", "#000000", primary)
	}
	if secondary != "#6E6E6E" {
		t.Errorf("expected valid secondary input %q to be used, got %q", "#6E6E6E", secondary)
	}
}

// TestSettingsPaneViewHasGradientBorder is a regression test for the
// visual-effects feature: Settings previously rendered as plain unbordered
// text — it must now be wrapped in a gradient border like every other
// screen.
func TestSettingsPaneViewHasGradientBorder(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	s.Width = 40
	s.Height = 12

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := s.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
