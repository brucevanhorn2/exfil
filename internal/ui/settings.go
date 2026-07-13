package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// settingsField indexes the rows in the Settings screen, in Tab order.
type settingsField int

const (
	settingsFieldLingo settingsField = iota
	settingsFieldPrimary
	settingsFieldSecondary
	settingsFieldCount
)

// SettingsPane is the dedicated Settings screen: lingo pack (arrow-cycled)
// and primary/secondary theme colors (free-form hex text entry). Kept
// separate from AboutPane (read-only) for single responsibility.
type SettingsPane struct {
	packIndex      int
	focused        settingsField
	primaryInput   textinput.Model
	secondaryInput textinput.Model
	errMsg         string
	Width          int
	Height         int
}

func NewSettingsPane() *SettingsPane {
	primary := textinput.New()
	primary.Prompt = ""
	primary.CharLimit = 7

	secondary := textinput.New()
	secondary.Prompt = ""
	secondary.CharLimit = 7

	return &SettingsPane{
		primaryInput:   primary,
		secondaryInput: secondary,
	}
}

// ResetFromConfig populates the pane from the model's current settings.
// Called every time the Settings screen is opened, so it never shows stale
// edits left over from a previously cancelled visit.
func (s *SettingsPane) ResetFromConfig(lingo, primaryHex, secondaryHex string) {
	s.packIndex = 0
	for i, p := range i18n.Packs() {
		if p == lingo {
			s.packIndex = i
			break
		}
	}
	s.primaryInput.SetValue(primaryHex)
	s.secondaryInput.SetValue(secondaryHex)
	s.errMsg = ""
	s.focused = settingsFieldLingo
	s.refreshFocus()
}

func (s *SettingsPane) refreshFocus() {
	if s.focused == settingsFieldPrimary {
		s.primaryInput.Focus()
	} else {
		s.primaryInput.Blur()
	}
	if s.focused == settingsFieldSecondary {
		s.secondaryInput.Focus()
	} else {
		s.secondaryInput.Blur()
	}
}

// Focused reports which row currently has focus.
func (s *SettingsPane) Focused() settingsField {
	return s.focused
}

// NextField moves focus to the next row, wrapping around.
func (s *SettingsPane) NextField() {
	s.focused = (s.focused + 1) % settingsFieldCount
	s.refreshFocus()
}

// PrevField moves focus to the previous row, wrapping around.
func (s *SettingsPane) PrevField() {
	s.focused = (s.focused - 1 + settingsFieldCount) % settingsFieldCount
	s.refreshFocus()
}

// CyclePack moves the Lingo Pack selection by delta (+1 or -1), wrapping
// around. Only meaningful when the Lingo Pack row is focused — the caller
// (Model.handleSettingsKey) is responsible for only calling this when
// Focused() == settingsFieldLingo.
func (s *SettingsPane) CyclePack(delta int) {
	packs := i18n.Packs()
	n := len(packs)
	s.packIndex = ((s.packIndex+delta)%n + n) % n
}

// CurrentPack returns the currently-selected (not yet necessarily saved)
// lingo pack name.
func (s *SettingsPane) CurrentPack() string {
	return i18n.Packs()[s.packIndex]
}

// HandleKey forwards a key message to whichever color textinput has focus.
// A no-op if the Lingo Pack row is focused (it has no textinput).
func (s *SettingsPane) HandleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	switch s.focused {
	case settingsFieldPrimary:
		s.primaryInput, cmd = s.primaryInput.Update(msg)
	case settingsFieldSecondary:
		s.secondaryInput, cmd = s.secondaryInput.Update(msg)
	}
	return cmd
}

// PreviewColors returns the two color rows' current text if each is
// syntactically valid hex, else the corresponding fallback — so a live
// preview can hold the last valid color instead of erroring/flickering
// while the user is mid-edit.
func (s *SettingsPane) PreviewColors(fallbackPrimary, fallbackSecondary string) (string, string) {
	primary := fallbackPrimary
	if _, err := parseHexColor(s.primaryInput.Value()); err == nil {
		primary = s.primaryInput.Value()
	}
	secondary := fallbackSecondary
	if _, err := parseHexColor(s.secondaryInput.Value()); err == nil {
		secondary = s.secondaryInput.Value()
	}
	return primary, secondary
}

// Validate checks both color fields are valid hex. On failure, errMsg is
// set (surfaced by View()) and the same error is returned so the caller
// knows not to save/close.
func (s *SettingsPane) Validate() error {
	if _, err := parseHexColor(s.primaryInput.Value()); err != nil {
		s.errMsg = fmt.Sprintf("primary color: %v", err)
		return err
	}
	if _, err := parseHexColor(s.secondaryInput.Value()); err != nil {
		s.errMsg = fmt.Sprintf("secondary color: %v", err)
		return err
	}
	s.errMsg = ""
	return nil
}

func (s *SettingsPane) PrimaryValue() string   { return s.primaryInput.Value() }
func (s *SettingsPane) SecondaryValue() string { return s.secondaryInput.Value() }

func (s *SettingsPane) View(theme Theme, loc *i18n.Localizer) string {
	title := gradientText(loc.T("screen_title_settings"), theme.PrimaryColor, theme.SecondaryColor)

	rows := []string{title, ""}

	lingoStyle := theme.BrowserFile
	if s.focused == settingsFieldLingo {
		lingoStyle = theme.BrowserDir
	}
	rows = append(rows, lingoStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_lingo")))+" ◄ "+s.CurrentPack()+" ►")

	primaryStyle := theme.BrowserFile
	if s.focused == settingsFieldPrimary {
		primaryStyle = theme.BrowserDir
	}
	rows = append(rows, primaryStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_primary")))+" "+s.primaryInput.View())

	secondaryStyle := theme.BrowserFile
	if s.focused == settingsFieldSecondary {
		secondaryStyle = theme.BrowserDir
	}
	rows = append(rows, secondaryStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_secondary")))+" "+s.secondaryInput.View())

	if s.errMsg != "" {
		rows = append(rows, "", theme.StatusError.Render(loc.T("error_prefix")+s.errMsg))
	}

	rows = append(rows, "", theme.PaneTitle.Render(loc.T("settings_hint")))

	content := strings.Join(rows, "\n")
	return gradientBox(content, s.Width, s.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
