package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// hexToRGB parses a "#RRGGBB" string into its red/green/blue components.
// Callers only ever pass already-validated hex (via parseHexColor) or
// trusted internal constants (logoFrom/logoTo), so a parse failure here
// isn't a real runtime condition to recover from.
func hexToRGB(hex string) (r, g, b int) {
	_, _ = fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return
}

// lerp linearly interpolates between a and b at position t (0.0-1.0).
func lerp(a, b int, t float64) int {
	return int(float64(a) + t*float64(b-a))
}

// gradientLogo renders text with a horizontal color gradient from `from` to
// `to`, interpolated by each character's column position relative to the
// widest line, so the gradient flows consistently across the whole block
// rather than resetting on each line. Used by the About screen's ASCII logo
// (its own fixed cyan/purple endpoints, independent of the user's theme).
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

// gradientText applies the same horizontal gradient as gradientLogo to a
// single line of arbitrary text (e.g. a pane's title bar) — gradientLogo
// already handles a single-line input correctly (maxWidth becomes that
// line's own length), so this is a thin, typed wrapper rather than a
// reimplementation.
func gradientText(s string, from, to lipgloss.Color) string {
	return gradientLogo(s, string(from), string(to))
}

// mutedColor blends c 50% of the way toward black. Used for unfocused
// panes' gradient endpoints — there's no way to query the user's actual
// terminal background color, so black is a deterministic stand-in for
// "dimmer," not a literal background match.
func mutedColor(c lipgloss.Color) lipgloss.Color {
	r, g, b := hexToRGB(string(c))
	hex := fmt.Sprintf("#%02x%02x%02x", lerp(r, 0, 0.5), lerp(g, 0, 0.5), lerp(b, 0, 0.5))
	return lipgloss.Color(hex)
}

// gradientBox manually draws a rounded-corner border around content (built
// exactly as every pane already builds it — already-styled title/listing/
// etc. text, left untouched by this function), sized to width x height —
// the *interior* box size, matching lipgloss.Style's own Width()/Height()
// convention (total rendered size is width+2 columns by at least height+2
// rows). Each border character is colored by its position along the box's
// diagonal, from `from` at the top-left corner to `to` at the bottom-right.
//
// Width wraps overflowing lines (via lipgloss's own Width(), which also
// pads every resulting physical line to exactly the requested width while
// preserving embedded ANSI styling) — the same reflow every pane relied on
// pre-gradient, via a flat-colored `lipgloss.Style` with Width()/Border()
// set directly. Height is
// a floor, not a ceiling: content shorter than height is padded with blank
// rows, but content already taller is never truncated — matching
// lipgloss.Style.Height()'s real behavior (verified empirically: a
// Height(3) box given 6 lines of content renders 6 content rows, not 3).
func gradientBox(content string, width, height int, from, to lipgloss.Color) string {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	innerWidth := width - 2 // 1-space margin each side, like Padding(0, 1)
	if innerWidth < 0 {
		innerWidth = 0
	}

	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(content)
	lines := strings.Split(wrapped, "\n")

	blankRow := strings.Repeat(" ", innerWidth)
	for len(lines) < height {
		lines = append(lines, blankRow)
	}
	actualHeight := len(lines)

	fr, fg, fb := hexToRGB(string(from))
	tr, tg, tb := hexToRGB(string(to))
	denom := float64(width + actualHeight + 2)

	colorAt := func(x, y int) lipgloss.Color {
		t := float64(x+y) / denom
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		hex := fmt.Sprintf("#%02x%02x%02x", lerp(fr, tr, t), lerp(fg, tg, t), lerp(fb, tb, t))
		return lipgloss.Color(hex)
	}
	borderChar := func(x, y int, ch string) string {
		return lipgloss.NewStyle().Foreground(colorAt(x, y)).Render(ch)
	}

	rows := make([]string, 0, actualHeight+2)

	var top strings.Builder
	top.WriteString(borderChar(0, 0, "╭"))
	for x := 1; x <= width; x++ {
		top.WriteString(borderChar(x, 0, "─"))
	}
	top.WriteString(borderChar(width+1, 0, "╮"))
	rows = append(rows, top.String())

	for i, line := range lines {
		y := i + 1
		rows = append(rows, borderChar(0, y, "│")+" "+line+" "+borderChar(width+1, y, "│"))
	}

	bottomY := actualHeight + 1
	var bottom strings.Builder
	bottom.WriteString(borderChar(0, bottomY, "╰"))
	for x := 1; x <= width; x++ {
		bottom.WriteString(borderChar(x, bottomY, "─"))
	}
	bottom.WriteString(borderChar(width+1, bottomY, "╯"))
	rows = append(rows, bottom.String())

	return strings.Join(rows, "\n")
}
