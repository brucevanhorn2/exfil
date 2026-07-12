package transfer

import (
	"fmt"
	"io"
	"time"

	"github.com/bvanhorn/exfil/internal/fsys"
	tea "github.com/charmbracelet/bubbletea"
)

// progressWriter wraps an io.Writer and tracks bytes written, emitting progress messages.
// It throttles messages to ~6 per second (150ms intervals) to avoid flooding eventsCh
// on fast copies, and always emits when done. Used as:
//
//	io.Copy(dst, io.TeeReader(src, &progressWriter{...}))
type progressWriter struct {
	id        int
	total     int64
	written   int64
	lastSent  time.Time
	ch        chan<- tea.Msg
	startTime time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	now := time.Now()
	// Throttle messages: only send if 150ms has passed or we're at 100%.
	// This prevents fast local disk copies from generating thousands of messages.
	if now.Sub(pw.lastSent) > 150*time.Millisecond || pw.written == pw.total {
		pw.lastSent = now
		elapsed := now.Sub(pw.startTime).Seconds()
		var speed string
		if elapsed > 0 {
			bytesPerSec := float64(pw.written) / elapsed
			if bytesPerSec > 1024*1024 {
				speed = fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
			} else if bytesPerSec > 1024 {
				speed = fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
			} else {
				speed = fmt.Sprintf("%.1f B/s", bytesPerSec)
			}
		}
		pw.ch <- TransferProgressMsg{ID: pw.id, Done: pw.written, Total: pw.total, Speed: speed}
	}
	return n, nil
}

type TransferProgressMsg struct {
	ID    int
	Done  int64
	Total int64
	Speed string
}

type TransferDoneMsg struct {
	ID int
}

type TransferErrorMsg struct {
	ID  int
	Err error
}

// RunWithFS executes a transfer with explicit source and destination
// FileSystems. The worker pool calls this with the job's SrcFS/DstFS, so the
// same code path handles local copies, downloads (RemoteFS→LocalFS), and
// uploads (LocalFS→RemoteFS).
func RunWithFS(job Job, events chan<- tea.Msg, src fsys.FileSystem, dst fsys.FileSystem) {
	srcEntry, err := src.Stat(job.SourcePath)
	if err != nil {
		events <- TransferErrorMsg{ID: job.ID, Err: err}
		return
	}

	if srcEntry.IsDir {
		events <- TransferErrorMsg{ID: job.ID, Err: fmt.Errorf("directories not supported")}
		return
	}

	srcFile, err := src.Open(job.SourcePath)
	if err != nil {
		events <- TransferErrorMsg{ID: job.ID, Err: err}
		return
	}
	defer srcFile.Close()

	dstFile, err := dst.Create(job.DestPath)
	if err != nil {
		events <- TransferErrorMsg{ID: job.ID, Err: err}
		return
	}
	defer dstFile.Close()

	pw := &progressWriter{
		id:        job.ID,
		total:     srcEntry.Size,
		ch:        events,
		startTime: time.Now(),
	}

	_, err = io.Copy(dstFile, io.TeeReader(srcFile, pw))
	if err != nil {
		events <- TransferErrorMsg{ID: job.ID, Err: err}
		return
	}

	events <- TransferDoneMsg{ID: job.ID}
}
