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

func (c *ConfirmPane) View(theme Theme, loc *i18n.Localizer) string {
	lines := []string{
		gradientText(loc.T("confirm_delete_header"), theme.PrimaryColor, theme.SecondaryColor),
		"",
		c.Message,
		"",
		theme.BrowserFile.Render(loc.T("confirm_delete_hint")),
	}
	content := strings.Join(lines, "\n")
	return gradientBox(content, c.Width, c.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
