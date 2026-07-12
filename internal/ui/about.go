package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/version"
	"github.com/charmbracelet/lipgloss"
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

// gradientLogo renders text with a horizontal color gradient from `from` to
// `to`, interpolated by each character's column position relative to the
// widest line, so the gradient flows consistently across the whole block
// rather than resetting on each line.
func gradientLogo(text, from, to string) string {
	lines := strings.Split(text, "\n")

	maxWidth := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth <= 1 {
		return text
	}

	fr, fg, fb := hexToRGB(from)
	tr, tg, tb := hexToRGB(to)

	out := make([]string, len(lines))
	for li, line := range lines {
		var b strings.Builder
		for i, r := range []rune(line) {
			if r == ' ' {
				b.WriteRune(r)
				continue
			}
			t := float64(i) / float64(maxWidth-1)
			hex := fmt.Sprintf("#%02x%02x%02x", lerp(fr, tr, t), lerp(fg, tg, t), lerp(fb, tb, t))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(string(r)))
		}
		out[li] = b.String()
	}
	return strings.Join(out, "\n")
}

func hexToRGB(hex string) (r, g, b int) {
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return
}

func lerp(a, b int, t float64) int {
	return int(float64(a) + t*float64(b-a))
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
	return theme.PaneBorderFocus.Width(a.Width).Height(a.Height).Render(content)
}
