package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
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
}

func NewQueuePane() *QueuePane {
	return &QueuePane{
		Transfers: []Transfer{},
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

func (q *QueuePane) View(theme Theme, loc *i18n.Localizer) string {
	title := theme.QueueTitle.Render(loc.T("screen_title_queue"))
	border := theme.QueueBorder

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
		lines = append(lines, loc.T("queue_empty"))
		rowsRendered++
	} else {
		for _, t := range transfers {
			lines = append(lines, q.renderTransfer(t, theme, loc))
			rowsRendered++
		}
	}

	for ; rowsRendered < maxRows; rowsRendered++ {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return border.Width(q.Width).Render(content)
}

func (q *QueuePane) renderTransfer(t Transfer, theme Theme, loc *i18n.Localizer) string {
	var statusKey string
	statusStyle := theme.TransferQueued

	switch t.Status {
	case StatusQueued:
		statusKey = "transfer_status_queued"
		statusStyle = theme.TransferQueued
	case StatusRunning:
		statusKey = "transfer_status_running"
		statusStyle = theme.TransferRunning
	case StatusDone:
		statusKey = "transfer_status_done"
		statusStyle = theme.TransferDone
	case StatusError:
		statusKey = "transfer_status_error"
		statusStyle = theme.TransferError
	}

	nameWidth := 20
	if len(t.Filename) > nameWidth {
		nameWidth = len(t.Filename)
	}

	name := fmt.Sprintf("%-"+fmt.Sprint(nameWidth)+"s", t.Filename)
	status := statusStyle.Render(fmt.Sprintf("%-10s", loc.T(statusKey)))

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
		line = theme.TransferError.Render(line + " (" + t.Error + ")")
	}

	return line
}
