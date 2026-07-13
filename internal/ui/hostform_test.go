package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func newTestForm() *HostFormPane {
	return NewHostFormPane()
}

func TestBuildHostValidation(t *testing.T) {
	tests := []struct {
		name, hostname, user, port string
		wantErr                    bool
	}{
		{name: "", hostname: "h", user: "u", port: "22", wantErr: true},
		{name: "n", hostname: "", user: "u", port: "22", wantErr: true},
		{name: "n", hostname: "h", user: "", port: "22", wantErr: true},
		{name: "n", hostname: "h", user: "u", port: "notanumber", wantErr: true},
		{name: "n", hostname: "h", user: "u", port: "0", wantErr: true},
		{name: "n", hostname: "h", user: "u", port: "70000", wantErr: true},
		{name: "n", hostname: "h", user: "u", port: "", wantErr: false},
		{name: "n", hostname: "h", user: "u", port: "2222", wantErr: false},
	}

	for _, tt := range tests {
		f := newTestForm()
		f.inputs[fieldName].SetValue(tt.name)
		f.inputs[fieldHostname].SetValue(tt.hostname)
		f.inputs[fieldUser].SetValue(tt.user)
		f.inputs[fieldPort].SetValue(tt.port)

		_, err := f.buildHost(i18n.NewLocalizer("plain"))
		if (err != nil) != tt.wantErr {
			t.Errorf("name=%q hostname=%q user=%q port=%q: got err=%v, wantErr=%v",
				tt.name, tt.hostname, tt.user, tt.port, err, tt.wantErr)
		}
	}
}

func TestBuildHostDefaultPort(t *testing.T) {
	f := newTestForm()
	f.inputs[fieldName].SetValue("n")
	f.inputs[fieldHostname].SetValue("h")
	f.inputs[fieldUser].SetValue("u")

	host, err := f.buildHost(i18n.NewLocalizer("plain"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host.Port != config.DefaultPort() {
		t.Errorf("expected default port %d, got %d", config.DefaultPort(), host.Port)
	}
}

// TestSaveEditsByName is a regression test: edits must be keyed by the
// host's original Name, not by list position, so a stale index can't
// silently overwrite the wrong entry (see hostform.go's Save comment).
func TestSaveEditsByName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	seed := &config.Config{Hosts: []config.Host{
		{Name: "alpha", Hostname: "1.1.1.1", Port: 22, User: "a"},
		{Name: "beta", Hostname: "2.2.2.2", Port: 22, User: "b"},
	}}
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}

	f := newTestForm()
	f.ResetForEdit(seed.Hosts[1]) // editing "beta"
	f.inputs[fieldHostname].SetValue("3.3.3.3")

	if _, err := f.Save(i18n.NewLocalizer("plain")); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(got.Hosts))
	}
	if got.Hosts[0].Name != "alpha" || got.Hosts[0].Hostname != "1.1.1.1" {
		t.Errorf("alpha host was unexpectedly modified: %+v", got.Hosts[0])
	}

	var beta *config.Host
	for i := range got.Hosts {
		if got.Hosts[i].Name == "beta" {
			beta = &got.Hosts[i]
		}
	}
	if beta == nil {
		t.Fatal("beta host missing after save")
	}
	if beta.Hostname != "3.3.3.3" {
		t.Errorf("expected beta hostname updated to 3.3.3.3, got %s", beta.Hostname)
	}
}

func TestSaveAddsNewHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	f := newTestForm()
	f.ResetForAdd()
	f.inputs[fieldName].SetValue("newhost")
	f.inputs[fieldHostname].SetValue("9.9.9.9")
	f.inputs[fieldUser].SetValue("u")

	if _, err := f.Save(i18n.NewLocalizer("plain")); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 1 || got.Hosts[0].Name != "newhost" {
		t.Errorf("expected one host named newhost, got %+v", got.Hosts)
	}
}

// TestHostFormPaneViewHasGradientBorder is a regression test for the
// visual-effects feature: Add/Edit Host previously rendered as plain
// unbordered text — it must now show an actual color gradient border.
func TestHostFormPaneViewHasGradientBorder(t *testing.T) {
	f := newTestForm()
	f.ResetForAdd()
	f.Width = 40
	f.Height = 14

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := f.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
