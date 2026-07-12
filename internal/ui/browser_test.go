package ui

import (
	"fmt"
	"testing"

	"github.com/bvanhorn/exfil/internal/fsys"
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
		b := NewBrowserPane("test", fsys.LocalFS{}, NewTheme())
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
	b := NewBrowserPane("test", fsys.LocalFS{}, NewTheme())
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
