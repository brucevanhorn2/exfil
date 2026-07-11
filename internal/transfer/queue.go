package transfer

import (
	"github.com/bvanhorn/exfil/internal/fsys"
	tea "github.com/charmbracelet/bubbletea"
)

// StartWorkers spawns n goroutines, each pulling jobs from the jobs channel
// and executing them via RunWithFS(). This bounds concurrency to n — only n
// goroutines will be active at a time, pulling jobs in FIFO order.
// Workers send progress/done/error messages on the events channel for the UI to display.
// Workers run for the lifetime of the program (blocking on channel recv).
func StartWorkers(n int, jobs <-chan Job, events chan<- tea.Msg) {
	for i := 0; i < n; i++ {
		go func() {
			// Each worker loops indefinitely, pulling jobs off the channel.
			// The job carries its own source/destination filesystems (local or
			// remote), so the same worker handles local copies, downloads, and
			// uploads without knowing which is which. nil FS falls back to local.
			for job := range jobs {
				src := job.SrcFS
				if src == nil {
					src = fsys.LocalFS{}
				}
				dst := job.DstFS
				if dst == nil {
					dst = fsys.LocalFS{}
				}
				RunWithFS(job, events, src, dst)
			}
		}()
	}
}
