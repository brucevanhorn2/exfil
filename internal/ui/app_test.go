package ui

import (
	"log"
	"os"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
}

func TestNewModelDefaultsWhenConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger())

	if m.loc.Pack() != "plain" {
		t.Errorf("expected default pack \"plain\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected default primary color %q, got %q", DefaultPrimaryColor, m.primaryColorHex)
	}
	if m.secondaryColorHex != DefaultSecondaryColor {
		t.Errorf("expected default secondary color %q, got %q", DefaultSecondaryColor, m.secondaryColorHex)
	}
}

func TestNewModelUsesSavedLingoAndColors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{Lingo: "keyboardcowboy", PrimaryColor: "#39FF14", SecondaryColor: "#3A3A4A"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger())

	if m.loc.Pack() != "keyboardcowboy" {
		t.Errorf("expected pack \"keyboardcowboy\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != "#39FF14" {
		t.Errorf("expected primary color #39FF14, got %q", m.primaryColorHex)
	}
}

func TestNewModelFallsBackOnInvalidStoredColor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{PrimaryColor: "not-a-color"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger())

	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected fallback to default primary color for invalid stored value, got %q", m.primaryColorHex)
	}
}
