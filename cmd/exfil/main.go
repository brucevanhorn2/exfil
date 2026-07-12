package main

import (
	"flag"
	"log"
	"os"

	"github.com/bvanhorn/exfil/internal/transfer"
	"github.com/bvanhorn/exfil/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	testMode := flag.Bool("t", false, "test mode: show the local filesystem in both panes without an SSH connection, for local-to-local transfer testing")
	flag.Parse()

	f, err := os.OpenFile("/tmp/exfil.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("failed to open log: %v", err)
	}
	defer f.Close()

	logger := log.New(f, "", log.LstdFlags)

	// eventsCh: Worker goroutines send progress/done/error messages here.
	// Buffered to avoid blocking workers, UI drains it via subscription (re-arming tea.Cmd).
	eventsCh := make(chan tea.Msg, 64)

	// jobsCh: UI sends transfer jobs here. Workers pull from this channel.
	// This bounds concurrency to N workers — only N goroutines pull from the channel.
	jobsCh := make(chan transfer.Job, 256)

	// Start 3 worker goroutines, each pulling jobs and sending progress on eventsCh.
	// Workers run for the lifetime of the program (blocking on channel recv).
	transfer.StartWorkers(3, jobsCh, eventsCh)

	// Pass both channels to the UI model.
	model := ui.NewModel(eventsCh, jobsCh, logger, *testMode)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		log.Fatalf("error running program: %v", err)
	}
}
