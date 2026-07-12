package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
)

type HostPickerPane struct {
	Hosts  []config.Host
	Cursor int
	Focus  bool
	Width  int
	Height int
}

func NewHostPickerPane() *HostPickerPane {
	return &HostPickerPane{
		Hosts: []config.Host{},
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

func (hp *HostPickerPane) View(theme Theme, loc *i18n.Localizer) string {
	if len(hp.Hosts) == 0 {
		return theme.PaneTitle.Render(loc.T("hostpicker_empty"))
	}

	lines := []string{
		theme.PaneTitle.Render(loc.T("hostpicker_header")),
	}

	for i, host := range hp.Hosts {
		prefix := "  "
		style := theme.BrowserFile
		if i == hp.Cursor {
			prefix = "► "
			style = theme.BrowserDir
		}
		line := prefix + style.Render(host.Name) + " (" + host.User + "@" + host.Hostname + ")"
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
