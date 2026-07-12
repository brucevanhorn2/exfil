package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

// TestQueuePaneViewCapsHeight is a regression test: the queue pane used to
// grow by one rendered line per queued transfer with no limit, which could
// push the whole TUI taller than the terminal and scroll the top off-screen.
// View() must always render exactly q.Height lines regardless of how many
// transfers are queued.
func TestQueuePaneViewCapsHeight(t *testing.T) {
	q := NewQueuePane()
	// Wide enough that the rendered status word ("queued", "processing",
	// "in transit", etc., padded to 10 chars per renderTransfer) plus the
	// name/progress/size columns doesn't trigger lipgloss's word-wrap inside
	// the border — this test is about height capping, not line wrapping.
	q.Width = 100
	q.Height = 8

	for i := 0; i < 10; i++ {
		q.AddTransfer(Transfer{ID: i, Filename: "f", Status: StatusQueued})
	}

	view := q.View(NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor)), i18n.NewLocalizer("plain"))
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected view to render exactly %d lines, got %d", q.Height, len(lines))
	}
}

func TestQueuePaneViewEmptyFillsHeight(t *testing.T) {
	q := NewQueuePane()
	q.Width = 40
	q.Height = 8

	view := q.View(NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor)), i18n.NewLocalizer("plain"))
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected empty view to still render exactly %d lines, got %d", q.Height, len(lines))
	}
}
