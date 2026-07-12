package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bvanhorn/exfil/internal/fsys"
)

type BrowserPane struct {
	Title     string
	FS        fsys.FileSystem
	Cwd       string
	Entries   []fsys.Entry
	Cursor    int
	Focus     bool
	Selected  map[string]bool
	Width     int
	Height    int
	theme     Theme
	scrollTop int
}

func NewBrowserPane(title string, fs fsys.FileSystem, theme Theme) *BrowserPane {
	return &BrowserPane{
		Title:    title,
		FS:       fs,
		Cwd:      "/",
		Selected: make(map[string]bool),
		theme:    theme,
	}
}

func (b *BrowserPane) SetFocus(focus bool) {
	b.Focus = focus
	if focus && b.Cursor >= len(b.Entries) && len(b.Entries) > 0 {
		b.Cursor = 0
	}
}

func (b *BrowserPane) Refresh() error {
	entries, err := b.FS.ReadDir(b.Cwd)
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	b.SetEntries(entries)
	return nil
}

// SetEntries replaces the pane's listing and resets cursor, scroll, and
// selection. Used both by Refresh (synchronous) and by the async readDirCmd
// path (remote listings loaded off the UI thread), so state stays consistent.
func (b *BrowserPane) SetEntries(entries []fsys.Entry) {
	b.Entries = entries
	b.Cursor = 0
	b.Selected = make(map[string]bool)
	b.scrollTop = 0
}

func (b *BrowserPane) Up() {
	if b.Cursor > 0 {
		b.Cursor--
	}
	b.ensureVisible()
}

func (b *BrowserPane) Down() {
	if b.Cursor < len(b.Entries)-1 {
		b.Cursor++
	}
	b.ensureVisible()
}

func (b *BrowserPane) ensureVisible() {
	// Must match View()'s contentHeight (b.Height minus border top/bottom
	// and the title line) or the cursor can scroll past what's rendered.
	visibleRows := b.Height - 3
	if b.Cursor < b.scrollTop {
		b.scrollTop = b.Cursor
	}
	if b.Cursor >= b.scrollTop+visibleRows {
		b.scrollTop = b.Cursor - visibleRows + 1
	}
}

func (b *BrowserPane) Enter() error {
	if b.Cursor < 0 || b.Cursor >= len(b.Entries) {
		return nil
	}
	e := b.Entries[b.Cursor]
	if e.IsDir {
		b.Cwd = b.FS.Join(b.Cwd, e.Name)
		return b.Refresh()
	}
	return nil
}

func (b *BrowserPane) Back() error {
	if b.Cwd == "/" {
		return nil
	}
	// Splitting an absolute path always yields a leading "" element (from
	// the root slash), so parts[:len(parts)-1] already starts with "/" once
	// joined — prepending another "/" produced a "//" prefix.
	parts := strings.Split(strings.TrimRight(b.Cwd, "/"), "/")
	if len(parts) > 2 {
		b.Cwd = strings.Join(parts[:len(parts)-1], "/")
	} else {
		b.Cwd = "/"
	}
	return b.Refresh()
}

func (b *BrowserPane) ToggleSelect() {
	if b.Cursor < 0 || b.Cursor >= len(b.Entries) {
		return
	}
	e := b.Entries[b.Cursor]
	b.Selected[e.Name] = !b.Selected[e.Name]
}

func (b *BrowserPane) GetSelectedFiles() []string {
	var result []string
	for name, selected := range b.Selected {
		if selected {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func (b *BrowserPane) CurrentFile() *fsys.Entry {
	if b.Cursor < 0 || b.Cursor >= len(b.Entries) {
		return nil
	}
	return &b.Entries[b.Cursor]
}

func (b *BrowserPane) View() string {
	titleStyle := b.theme.PaneTitle
	borderStyle := b.theme.PaneBorder

	if b.Focus {
		titleStyle = b.theme.PaneTitleFocus
		borderStyle = b.theme.PaneBorderFocus
	}

	titleWithPath := titleStyle.Render(fmt.Sprintf(" %s:%s ", b.Title, b.Cwd))

	lines := []string{titleWithPath}

	// -2 for the border's top/bottom lines, -1 for the title line above.
	contentHeight := b.Height - 3
	if contentHeight < 0 {
		contentHeight = 0
	}

	rowsRendered := 0
	for i := b.scrollTop; i < len(b.Entries) && i < b.scrollTop+contentHeight; i++ {
		e := b.Entries[i]
		isCursor := i == b.Cursor && b.Focus
		isSelected := b.Selected[e.Name]

		// Cursor and selection are independent states and must both be
		// visible at once: a checkbox for selection, an arrow for cursor
		// position, so a selected file under the cursor doesn't look
		// identical to a merely-hovered one.
		cursorMark := " "
		if isCursor {
			cursorMark = "►"
		}
		selectMark := " "
		if isSelected {
			selectMark = "☑"
		}
		marker := cursorMark + selectMark + " "

		style := b.theme.BrowserFile
		if e.IsDir {
			style = b.theme.BrowserDir
		}
		if isSelected {
			style = b.theme.BrowserSelected
		}

		line := marker + style.Render(e.Name)
		if e.IsDir {
			line += "/"
		}

		lines = append(lines, line)
		rowsRendered++
	}

	// Pad remaining rows so the box fills its assigned height even when
	// there are fewer entries than contentHeight (e.g. a freshly-connected
	// remote pane with a short directory listing).
	for ; rowsRendered < contentHeight; rowsRendered++ {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	bordered := borderStyle.Width(b.Width).Render(content)
	return bordered
}
