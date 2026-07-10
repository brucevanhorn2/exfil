package transfer

import (
	tea "github.com/charmbracelet/bubbletea"
)

func StartWorkers(n int, jobs <-chan Job, events chan<- tea.Msg) {
	for i := 0; i < n; i++ {
		go func() {
			for job := range jobs {
				Run(job, events)
			}
		}()
	}
}
