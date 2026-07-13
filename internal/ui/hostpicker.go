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
	var content string
	if len(hp.Hosts) == 0 {
		content = gradientText(loc.T("hostpicker_empty"), theme.PrimaryColor, theme.SecondaryColor)
	} else {
		lines := []string{
			gradientText(loc.T("hostpicker_header"), theme.PrimaryColor, theme.SecondaryColor),
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

		content = strings.Join(lines, "\n")
	}

	return gradientBox(content, hp.Width, hp.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
