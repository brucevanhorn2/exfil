package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
)

type HostPickerPane struct {
	Hosts  []config.Host
	Cursor int
	Focus  bool
	Width  int
	Height int
	theme  Theme
}

func NewHostPickerPane(theme Theme) *HostPickerPane {
	return &HostPickerPane{
		Hosts:  []config.Host{},
		theme:  theme,
	}
}

func (hp *HostPickerPane) Load() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hp.Hosts = cfg.Hosts
	return nil
}

func (hp *HostPickerPane) Up() {
	if hp.Cursor > 0 {
		hp.Cursor--
	}
}

func (hp *HostPickerPane) Down() {
	if hp.Cursor < len(hp.Hosts)-1 {
		hp.Cursor++
	}
}

func (hp *HostPickerPane) CurrentHost() *config.Host {
	if hp.Cursor < 0 || hp.Cursor >= len(hp.Hosts) {
		return nil
	}
	return &hp.Hosts[hp.Cursor]
}

func (hp *HostPickerPane) View() string {
	if len(hp.Hosts) == 0 {
		return hp.theme.PaneTitle.Render(" No hosts configured. Press [n] to add a host. ")
	}

	lines := []string{
		hp.theme.PaneTitle.Render(" Saved Hosts - [↑/↓] navigate, [↵] connect, [n] add host "),
	}

	for i, host := range hp.Hosts {
		prefix := "  "
		style := hp.theme.BrowserFile
		if i == hp.Cursor {
			prefix = "► "
			style = hp.theme.BrowserDir
		}
		line := prefix + style.Render(host.Name) + " (" + host.User + "@" + host.Hostname + ")"
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
