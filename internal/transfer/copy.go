package transfer

import (
	"fmt"
	"io"
	"time"

	"github.com/bvanhorn/exfil/internal/fsys"
	tea "github.com/charmbracelet/bubbletea"
)

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

func Run(job Job, events chan<- tea.Msg) {
	src := fsys.LocalFS{}
	dst := fsys.LocalFS{}

	srcEntry, err := src.Stat(job.SourcePath)
	if err != nil {
		events <- TransferErrorMsg{ID: job.ID, Err: err}
		return
	}

	if srcEntry.IsDir {
		events <- TransferErrorMsg{ID: job.ID, Err: fmt.Errorf("directories not supported")}
		return
	}

	events <- struct {
		ID       int
		Filename string
		Total    int64
	}{ID: job.ID, Filename: job.Filename, Total: srcEntry.Size}

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
