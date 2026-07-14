package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
)

// ConfirmPane renders the Y/N delete confirmation screen (issue #4).
type ConfirmPane struct {
	Message string
	Width   int
	Height  int
}

// View takes headerKey as a parameter (derived by the caller from Model
// state, same convention as PromptPane.View) rather than storing it as a
// field — issue #15's recursive-delete escalation picks between
// "confirm_delete_header" and "confirm_delete_recursive_header" this way.
func (c *ConfirmPane) View(theme Theme, loc *i18n.Localizer, headerKey string) string {
	lines := []string{
		gradientText(loc.T(headerKey), theme.PrimaryColor, theme.SecondaryColor),
		"",
		c.Message,
		"",
		theme.BrowserFile.Render(loc.T("confirm_delete_hint")),
	}
	content := strings.Join(lines, "\n")
	return gradientBox(content, c.Width, c.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
