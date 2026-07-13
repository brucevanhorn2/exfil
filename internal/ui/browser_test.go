package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/charmbracelet/lipgloss"
)

// TestBrowserPaneBack is a regression test for a bug where Back() produced
// a doubled leading slash (e.g. "/home/bruce" -> "//home" instead of
// "/home"), because splitting an absolute path already yields a leading ""
// element that was then prefixed with another "/".
func TestBrowserPaneBack(t *testing.T) {
	tests := []struct {
		start, want string
	}{
		{"/home/bruce", "/home"},
		{"/home", "/"},
		{"/", "/"},
		{"/a/b/c", "/a/b"},
		{"/a", "/"},
	}

	for _, tt := range tests {
		b := NewBrowserPane("test", fsys.LocalFS{})
		b.Cwd = tt.start
		// Ignore the error: Refresh() may fail to ReadDir a path that
		// doesn't exist on the test runner, but Cwd is updated regardless
		// and that's the only thing this test cares about.
		_ = b.Back()
		if b.Cwd != tt.want {
			t.Errorf("Back() from %q: got Cwd=%q, want %q", tt.start, b.Cwd, tt.want)
		}
	}
}

// TestBrowserPaneEnsureVisible is a regression test for an off-by-one where
// ensureVisible()'s scroll math didn't match View()'s actual visible row
// count, letting the cursor scroll one row past what was rendered.
func TestBrowserPaneEnsureVisible(t *testing.T) {
	b := NewBrowserPane("test", fsys.LocalFS{})
	b.Height = 10 // contentHeight = 10-3 = 7 visible rows

	entries := make([]fsys.Entry, 20)
	for i := range entries {
		entries[i] = fsys.Entry{Name: fmt.Sprintf("file%d", i)}
	}
	b.SetEntries(entries)

	for i := 0; i < len(entries)-1; i++ {
		b.Down()
	}

	if b.Cursor != len(entries)-1 {
		t.Fatalf("expected cursor at %d, got %d", len(entries)-1, b.Cursor)
	}

	visibleRows := b.Height - 3
	if b.Cursor < b.scrollTop || b.Cursor >= b.scrollTop+visibleRows {
		t.Errorf("cursor %d not within visible window [%d, %d)", b.Cursor, b.scrollTop, b.scrollTop+visibleRows)
	}
}

// TestBrowserPaneEmptyMessageRendersWhenNoEntries is a regression test for
// the "remote pane silently shows the local filesystem before connecting"
// bug: when EmptyMessage is set and there are no entries, it must render
// instead of an unlabeled blank pane.
func TestBrowserPaneEmptyMessageRendersWhenNoEntries(t *testing.T) {
	b := NewBrowserPane("remote", fsys.LocalFS{})
	b.Width = 40
	b.Height = 10
	b.EmptyMessage = "Not connected. Press [s] to select a host."

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := b.View(theme)

	if !strings.Contains(view, "Not connected") {
		t.Errorf("expected EmptyMessage to render when there are no entries, got:\n%s", view)
	}
}

// TestBrowserPaneEmptyMessageHiddenOnceEntriesExist confirms EmptyMessage is
// purely a placeholder for the empty state — it must not linger once real
// entries are set (e.g. after connecting and listing a directory).
func TestBrowserPaneEmptyMessageHiddenOnceEntriesExist(t *testing.T) {
	b := NewBrowserPane("remote", fsys.LocalFS{})
	b.Width = 40
	b.Height = 10
	b.EmptyMessage = "Not connected. Press [s] to select a host."
	b.SetEntries([]fsys.Entry{{Name: "file.txt"}})

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := b.View(theme)

	if strings.Contains(view, "Not connected") {
		t.Errorf("EmptyMessage should not render once entries are present, got:\n%s", view)
	}
}

// TestBrowserPaneFocusUsesVividGradientUnfocusedUsesMuted is a regression
// test for the visual-effects feature: a focused pane's border/title must
// render with the full-intensity primary/secondary gradient, and an
// unfocused pane with the muted (50%-toward-black) variant — proving the
// two are visually distinguishable, not just structurally different.
func TestBrowserPaneFocusUsesVividGradientUnfocusedUsesMuted(t *testing.T) {
	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))

	b := NewBrowserPane("test", fsys.LocalFS{})
	b.Width = 30
	b.Height = 10

	b.Focus = false
	unfocused := b.View(theme)
	vividRed := "38;2;255;0;0"
	if strings.Contains(unfocused, vividRed) {
		t.Errorf("unfocused pane should not use the vivid primary color, got:\n%s", unfocused)
	}

	b.Focus = true
	focused := b.View(theme)
	if !strings.Contains(focused, vividRed) {
		t.Errorf("focused pane's title/border should include the vivid primary color, got:\n%s", focused)
	}
}

// TestBrowserPaneRenderedWidthMatchesAssignedWidth is a regression test for
// the width parameter bug: gradientBox expects interior width (total rendered
// width is width+2), so browser.go must pass b.Width, not b.Width-2. This
// test verifies that each rendered line has exactly b.Width+2 columns,
// matching the pane's assigned layout budget unchanged from before the
// visual-effects feature.
func TestBrowserPaneRenderedWidthMatchesAssignedWidth(t *testing.T) {
	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))

	b := NewBrowserPane("local", fsys.LocalFS{})
	b.Width = 40
	b.Height = 10
	b.SetEntries([]fsys.Entry{
		{Name: "file1.txt", IsDir: false},
		{Name: "file2.txt", IsDir: false},
		{Name: "directory", IsDir: true},
	})

	view := b.View(theme)
	lines := strings.Split(view, "\n")

	expectedWidth := b.Width + 2 // interior width + 2 border columns
	for i, line := range lines {
		actualWidth := lipgloss.Width(line)
		if actualWidth != expectedWidth {
			t.Errorf("line %d: visible width = %d, want %d (b.Width=%d plus 2 for borders)\n%q", i, actualWidth, expectedWidth, b.Width, line)
		}
	}
}
