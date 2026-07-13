package ui

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

// DefaultPrimaryColor and DefaultSecondaryColor are used when hosts.yaml has
// no saved color choices yet (a fresh install) or an invalid one (hand-edited
// YAML) — matching exfil's original magenta/gray look, but as precise
// true-color hex instead of a 16-color ANSI approximation.
const (
	DefaultPrimaryColor   = "#B341F5"
	DefaultSecondaryColor = "#6E6E6E"
)

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// parseHexColor validates s as a "#RRGGBB" hex color and returns it as a
// lipgloss.Color. Used for user-supplied colors (Settings screen input, or
// values loaded from hosts.yaml) so invalid input can be caught and reported
// rather than silently misrendering.
func parseHexColor(s string) (lipgloss.Color, error) {
	if !hexColorPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a valid hex color (expected format #RRGGBB)", s)
	}
	return lipgloss.Color(s), nil
}

type Theme struct {
	// Raw color values, needed by gradientBox/gradientText (a lipgloss.Style
	// only holds one flat color, not enough to interpolate a gradient from).
	PrimaryColor        lipgloss.Color
	SecondaryColor      lipgloss.Color
	MutedPrimaryColor   lipgloss.Color // PrimaryColor blended 50% toward black
	MutedSecondaryColor lipgloss.Color // SecondaryColor blended 50% toward black

	// Pane titles (borders are drawn by gradientBox using PrimaryColor/
	// SecondaryColor/Muted* above, not a lipgloss.Style)
	PaneTitle      lipgloss.Style
	PaneTitleFocus lipgloss.Style

	// Browser content
	BrowserDir      lipgloss.Style
	BrowserFile     lipgloss.Style
	BrowserSelected lipgloss.Style
	BrowserCursor   lipgloss.Style

	// Queue pane (border is drawn by gradientBox; title uses gradientText)
	TransferQueued  lipgloss.Style
	TransferRunning lipgloss.Style
	TransferDone    lipgloss.Style
	TransferError   lipgloss.Style
	ProgressBar     lipgloss.Style

	// Status bar
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusValue lipgloss.Style
	StatusError lipgloss.Style
}

// NewTheme builds the theme from a user-selectable primary color (focus
// borders/titles, selection background, progress bar, status values) and
// secondary color (unfocused borders/titles, queue chrome, queued-status
// text). Cyan/green/red stay fixed here since they carry meaning — running,
// done, and error — rather than being purely aesthetic.
func NewTheme(primary, secondary lipgloss.Color) Theme {
	return Theme{
		PrimaryColor:        primary,
		SecondaryColor:      secondary,
		MutedPrimaryColor:   mutedColor(primary),
		MutedSecondaryColor: mutedColor(secondary),

		PaneTitle: lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true),

		PaneTitleFocus: lipgloss.NewStyle().
			Foreground(primary).
			Bold(true),

		// Browser items
		BrowserDir: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		BrowserFile: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		BrowserSelected: lipgloss.NewStyle().
			Background(primary).
			Foreground(lipgloss.Color("0")),

		BrowserCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		// Queue
		TransferQueued: lipgloss.NewStyle().
			Foreground(secondary),

		TransferRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")),

		TransferDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")),

		TransferError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),

		ProgressBar: lipgloss.NewStyle().
			Foreground(primary),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		StatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		StatusValue: lipgloss.NewStyle().
			Foreground(primary),

		StatusError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),
	}
}
