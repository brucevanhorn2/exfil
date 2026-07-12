package ui

import (
	"log"
	"os"
	"path/filepath"
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

// TestHandleSettingsKeyEnterAbortsSaveOnConfigLoadFailure guards against a
// data-loss bug: if hosts.yaml is present but fails to parse when the
// Settings screen saves, handleSettingsKey must NOT call cfg.Save() with a
// freshly-zeroed Config, which would silently wipe the existing Hosts list.
// It must instead abort and surface an error, matching HostFormPane.Save().
func TestHandleSettingsKeyEnterAbortsSaveOnConfigLoadFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger())
	m.settingsPane.ResetFromConfig(m.loc.Pack(), m.primaryColorHex, m.secondaryColorHex)
	m.screen = ScreenSettings

	// Corrupt hosts.yaml after the Model has already loaded it once, so
	// config.Load() fails specifically inside the settings-save path.
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("hosts: [this is not valid: yaml\n")
	if err := os.WriteFile(p, corrupt, 0600); err != nil {
		t.Fatal(err)
	}

	newModel, _ := m.handleSettingsKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := newModel.(*Model)

	if m2.screen != ScreenBrowsing {
		t.Errorf("expected screen to return to Browsing even on save failure, got %v", m2.screen)
	}
	if m2.statusMsg == "" || m2.statusMsg == "Ready." {
		t.Errorf("expected an error status message, got %q", m2.statusMsg)
	}

	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(corrupt) {
		t.Errorf("hosts.yaml was overwritten despite config.Load() failure; got %q, want unchanged %q", after, corrupt)
	}
}
