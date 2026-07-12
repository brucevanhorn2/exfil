package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
)

type TransferStatus string

const (
	StatusQueued  TransferStatus = "queued"
	StatusRunning TransferStatus = "running"
	StatusDone    TransferStatus = "done"
	StatusError   TransferStatus = "error"
)

type Transfer struct {
	ID       int
	Filename string
	Status   TransferStatus
	Done     int64
	Total    int64
	Speed    string
	Error    string
}

type QueuePane struct {
	Transfers []Transfer
	Height    int
	Width     int
	theme     Theme
}

func NewQueuePane(theme Theme) *QueuePane {
	return &QueuePane{
		Transfers: []Transfer{},
		theme:     theme,
	}
}

func (q *QueuePane) AddTransfer(t Transfer) {
	q.Transfers = append(q.Transfers, t)
}

func (q *QueuePane) UpdateTransfer(id int, status TransferStatus, done, total int64, speed, errStr string) {
	for i, t := range q.Transfers {
		if t.ID == id {
			q.Transfers[i].Status = status
			q.Transfers[i].Done = done
			q.Transfers[i].Total = total
			q.Transfers[i].Speed = speed
			q.Transfers[i].Error = errStr
			break
		}
	}
}

func (q *QueuePane) View() string {
	title := q.theme.QueueTitle.Render(" Transfer Queue ")
	border := q.theme.QueueBorder

	// -2 for the border's top/bottom lines, -1 for the title line above.
	maxRows := q.Height - 3
	if maxRows < 1 {
		maxRows = 1
	}

	// Cap how many transfers are shown so the pane's rendered height never
	// exceeds its assigned budget (previously it grew by one line per queued
	// file with no limit, pushing the whole TUI taller than the terminal and
	// causing the top to scroll off). Show the most recent ones.
	transfers := q.Transfers
	if len(transfers) > maxRows {
		transfers = transfers[len(transfers)-maxRows:]
	}

	lines := []string{title}
	rowsRendered := 0

	if len(transfers) == 0 {
		lines = append(lines, "  No transfers")
		rowsRendered++
	} else {
		for _, t := range transfers {
			lines = append(lines, q.renderTransfer(t))
			rowsRendered++
		}
	}

	for ; rowsRendered < maxRows; rowsRendered++ {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return border.Width(q.Width).Render(content)
}

func (q *QueuePane) renderTransfer(t Transfer) string {
	statusStr := ""
	statusStyle := q.theme.TransferQueued

	switch t.Status {
	case StatusQueued:
		statusStr = "⏳ Q"
		statusStyle = q.theme.TransferQueued
	case StatusRunning:
		statusStr = "▶ ▶"
		statusStyle = q.theme.TransferRunning
	case StatusDone:
		statusStr = "✓ ✓"
		statusStyle = q.theme.TransferDone
	case StatusError:
		statusStr = "✗ ✗"
		statusStyle = q.theme.TransferError
	}

	nameWidth := 20
	if len(t.Filename) > nameWidth {
		nameWidth = len(t.Filename)
	}

	name := fmt.Sprintf("%-"+fmt.Sprint(nameWidth)+"s", t.Filename)
	status := statusStyle.Render(statusStr)

	var progressView string
	if t.Total > 0 {
		pct := float64(t.Done) / float64(t.Total)
		prog := progress.New(progress.WithScaledGradient("#ff00ff", "#00ffff"))
		progressView = prog.ViewAs(pct)
	} else {
		progressView = "      "
	}

	sizeStr := fmt.Sprintf("%d/%d", t.Done, t.Total)
	if len(sizeStr) < 15 {
		sizeStr = fmt.Sprintf("%-15s", sizeStr)
	}

	line := fmt.Sprintf("%s %s %s %s %s", status, name, progressView, sizeStr, t.Speed)

	if t.Error != "" {
		line = q.theme.TransferError.Render(line + " (" + t.Error + ")")
	}

	return line
}
