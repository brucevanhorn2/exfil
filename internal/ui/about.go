package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/version"
)

// logo is a "bigmono12"-style ASCII rendering of "exfil" (via
// `toilet -f bigmono12`), colored at render time with a gradient instead of
// baking in ANSI codes here.
const logo = `                                  ██
                       ▒████      ██     ████
                       █████      ██     ████
                       ██                  ██
  ░████▒   ███  ███  ███████    ████       ██
 ░██████▒   ██▒▒██   ███████    ████       ██
 ██▒  ▒██   ▒████▒     ██         ██       ██
 ████████    ████      ██         ██       ██
 ████████    ▒██▒      ██         ██       ██
 ██          ████      ██         ██       ██
 ███░  ▒█   ▒████▒     ██         ██       ██▒
 ░███████   ██▒▒██     ██      ████████    █████
  ░█████▒  ███  ███    ██      ████████    ░████  `

// logoFrom and logoTo are the gradient endpoints for the logo: a cyan on the
// left fading to a purple on the right, matching the app's cyberpunk accent
// colors but as true-color hex so the interpolation is smooth.
const (
	logoFrom = "#00E5FF"
	logoTo   = "#B341F5"
)

type AboutPane struct {
	Width  int
	Height int
}

func NewAboutPane() *AboutPane {
	return &AboutPane{}
}

func (a *AboutPane) View(theme Theme, loc *i18n.Localizer) string {
	lines := []string{
		gradientLogo(logo, logoFrom, logoTo),
		"",
		theme.BrowserFile.Render(loc.T("about_tagline")),
		"",
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_version"))) + version.Version,
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_license"))) + "MIT",
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_source"))) + "github.com/brucevanhorn2/exfil",
		"",
		theme.PaneTitle.Render(loc.T("about_close_hint")),
	}

	content := strings.Join(lines, "\n")
	// -2: gradientBox's height convention is interior rows only (a.Width
	// needs no such adjustment), matching every other pane's accounting.
	return gradientBox(content, a.Width, a.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
