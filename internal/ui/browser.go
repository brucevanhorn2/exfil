package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bvanhorn/exfil/internal/fsys"
)

type BrowserPane struct {
	Title       string
	FS          fsys.FileSystem
	Cwd         string
	Entries     []fsys.Entry
	Cursor      int
	Focus       bool
	Selected    map[string]bool
	Width       int
	Height      int
	theme       Theme
	scrollTop   int
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
	if b.Cursor < b.scrollTop {
		b.scrollTop = b.Cursor
	}
	if b.Cursor >= b.scrollTop+b.Height-2 {
		b.scrollTop = b.Cursor - b.Height + 2
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
	parts := strings.Split(strings.TrimRight(b.Cwd, "/"), "/")
	if len(parts) > 1 {
		b.Cwd = "/" + strings.Join(parts[:len(parts)-1], "/")
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

	contentHeight := b.Height - 2

	for i := b.scrollTop; i < len(b.Entries) && i < b.scrollTop+contentHeight; i++ {
		e := b.Entries[i]
		var line string

		if i == b.Cursor && b.Focus {
			marker := "► "
			if e.IsDir {
				line = marker + b.theme.BrowserDir.Render(e.Name) + "/"
			} else {
				line = marker + b.theme.BrowserFile.Render(e.Name)
			}
		} else if b.Selected[e.Name] {
			marker := "☑ "
			if e.IsDir {
				line = marker + b.theme.BrowserSelected.Render(e.Name) + "/"
			} else {
				line = marker + b.theme.BrowserSelected.Render(e.Name)
			}
		} else {
			marker := "  "
			if e.IsDir {
				line = marker + b.theme.BrowserDir.Render(e.Name) + "/"
			} else {
				line = marker + b.theme.BrowserFile.Render(e.Name)
			}
		}

		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	bordered := borderStyle.Render(content)
	return bordered
}
