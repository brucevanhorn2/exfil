package transfer

import (
	tea "github.com/charmbracelet/bubbletea"
)

// StartWorkers spawns n goroutines, each pulling jobs from the jobs channel
// and executing them via Run(). This bounds concurrency to n — only n goroutines
// will be active at a time, pulling jobs in FIFO order.
// Workers send progress/done/error messages on the events channel for the UI to display.
// Workers run for the lifetime of the program (blocking on channel recv).
func StartWorkers(n int, jobs <-chan Job, events chan<- tea.Msg) {
	for i := 0; i < n; i++ {
		go func() {
			// Each worker loops indefinitely, pulling jobs off the channel
			for job := range jobs {
				Run(job, events)
			}
		}()
	}
}
