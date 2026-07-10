package main

import (
	"log"
	"os"

	"github.com/bvanhorn/exfil/internal/transfer"
	"github.com/bvanhorn/exfil/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	f, err := os.OpenFile("/tmp/exfil.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("failed to open log: %v", err)
	}
	defer f.Close()

	logger := log.New(f, "", log.LstdFlags)

	eventsCh := make(chan tea.Msg, 64)
	jobsCh := make(chan transfer.Job, 256)

	transfer.StartWorkers(3, jobsCh, eventsCh)

	model := ui.NewModel(eventsCh, jobsCh, logger)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		log.Fatalf("error running program: %v", err)
	}
}
