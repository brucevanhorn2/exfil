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
