package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PromptPane is a single-field text input overlay shared by the rename and
// mkdir screens (issue #4) — both need identical single-line text entry,
// differing only in header text and what the caller does with the value.
type PromptPane struct {
	Input  textinput.Model
	ErrMsg string
	Width  int
	Height int
}

func NewPromptPane() *PromptPane {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	return &PromptPane{Input: ti}
}

// Reset seeds the input with value (the current name for rename; empty for
// mkdir), clears any previous error, and focuses the field.
func (p *PromptPane) Reset(value string) {
	p.Input.SetValue(value)
	p.Input.CursorEnd()
	p.ErrMsg = ""
	p.Input.Focus()
}

func (p *PromptPane) Value() string {
	return strings.TrimSpace(p.Input.Value())
}

func (p *PromptPane) HandleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	p.Input, cmd = p.Input.Update(msg)
	return cmd
}

func (p *PromptPane) View(theme Theme, loc *i18n.Localizer, headerKey string) string {
	lines := []string{
		gradientText(loc.T(headerKey), theme.PrimaryColor, theme.SecondaryColor),
		"",
		p.Input.View(),
	}
	if p.ErrMsg != "" {
		lines = append(lines, "", theme.StatusError.Render(loc.T("error_prefix")+p.ErrMsg))
	}
	content := strings.Join(lines, "\n")
	return gradientBox(content, p.Width, p.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
