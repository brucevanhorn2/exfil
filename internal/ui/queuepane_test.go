package ui

import (
	"strings"
	"testing"
)

// TestQueuePaneViewCapsHeight is a regression test: the queue pane used to
// grow by one rendered line per queued transfer with no limit, which could
// push the whole TUI taller than the terminal and scroll the top off-screen.
// View() must always render exactly q.Height lines regardless of how many
// transfers are queued.
func TestQueuePaneViewCapsHeight(t *testing.T) {
	q := NewQueuePane(NewTheme())
	q.Width = 40
	q.Height = 8

	for i := 0; i < 10; i++ {
		q.AddTransfer(Transfer{ID: i, Filename: "f", Status: StatusQueued})
	}

	view := q.View()
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected view to render exactly %d lines, got %d", q.Height, len(lines))
	}
}

func TestQueuePaneViewEmptyFillsHeight(t *testing.T) {
	q := NewQueuePane(NewTheme())
	q.Width = 40
	q.Height = 8

	view := q.View()
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected empty view to still render exactly %d lines, got %d", q.Height, len(lines))
	}
}
