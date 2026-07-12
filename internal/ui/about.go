package ui

import (
	"fmt"
	"strings"

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
	theme  Theme
	Width  int
	Height int
}

func NewAboutPane(theme Theme) *AboutPane {
	return &AboutPane{theme: theme}
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

func (a *AboutPane) View() string {
	lines := []string{
		gradientLogo(logo, logoFrom, logoTo),
		"",
		a.theme.BrowserFile.Render("cyberpunk TUI SCP/SFTP client"),
		"",
		a.theme.BrowserDir.Render("Version:  ") + version.Version,
		a.theme.BrowserDir.Render("License:  ") + "MIT",
		a.theme.BrowserDir.Render("Source:   ") + "github.com/brucevanhorn2/exfil",
		"",
		a.theme.PaneTitle.Render("[Esc/q] close"),
	}

	content := strings.Join(lines, "\n")
	return a.theme.PaneBorderFocus.Width(a.Width).Height(a.Height).Render(content)
}
