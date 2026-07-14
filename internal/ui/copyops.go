package ui

import (
	"github.com/bvanhorn/exfil/internal/fsys"
	"github.com/bvanhorn/exfil/internal/transfer"
)

// transferQueuedMsg tells Update() a file has been handed to the worker
// pool, so it can register the transfer in the queue pane and transferDest
// map — all Model mutation stays inside Update() (single-threaded), never
// in enqueueFileCopy/enqueueDirectoryCopy's caller goroutine, per the CSP
// discipline documented in CLAUDE.md.
type transferQueuedMsg struct {
	id       int
	filename string
	total    int64
	destPane string
}

// transferQueueErrorMsg reports a failure encountered while walking a
// directory (MkdirAll or ReadDir) — surfaced via m.statusMsg since it
// happens before any transfer.Job (and therefore any queue pane row) exists
// for it to attach to.
type transferQueueErrorMsg struct {
	label string
	err   error
}

// enqueueFileCopy allocates a transfer ID and hands the job to the worker
// pool. Safe to call from any goroutine: it only sends on m.eventsCh/
// m.jobsCh and calls the mutex-guarded allocateTransferID, never touching
// m.queuePane/m.statusMsg/m.nextID directly.
func (m *Model) enqueueFileCopy(srcFS, dstFS fsys.FileSystem, srcPath, dstPath, filename string, size int64, destPane string) {
	id := m.allocateTransferID()
	m.eventsCh <- transferQueuedMsg{id: id, filename: filename, total: size, destPane: destPane}
	m.jobsCh <- transfer.Job{
		ID:         id,
		SourcePath: srcPath,
		DestPath:   dstPath,
		Filename:   filename,
		SrcFS:      srcFS,
		DstFS:      dstFS,
	}
}

// enqueueDirectoryCopy walks srcRoot (already known to be a directory),
// mirroring its structure under dstRoot via MkdirAll, then enqueues one
// enqueueFileCopy per leaf file. It returns the number of files enqueued
// (recursively, including subdirectories), so the caller can tell whether
// anything will ever trigger a TransferDoneMsg-based destination-pane
// refresh.
//
// A MkdirAll/ReadDir failure on one subtree is reported and that subtree is
// skipped, without aborting siblings or other marked entries — matching the
// "skip and continue" decision for issue #6, distinct from delete's
// stop-at-first-error (a delete's target list is small and user-picked; a
// copy's file list can be huge and undiscovered until walked, so one bad
// subtree shouldn't sink the rest). label is a path relative to the copy's
// root (not just the bare directory name), so a nested failure is
// unambiguous even if two same-named subdirectories exist at different
// depths.
//
// dstFS.MkdirAll (rather than a plain Mkdir) costs a Stat round-trip more
// than strictly necessary here: by construction, dstRoot's parent was just
// created (or confirmed to exist) one recursion level up, so a plain Mkdir
// would always succeed. MkdirAll is used anyway for its "already exists"
// idempotency (re-running a copy into a partially-populated destination
// shouldn't error) without hand-rolling that check against LocalFS/
// RemoteFS's differently-shaped "already exists" errors — an accepted,
// minor inefficiency for deep/wide trees rather than a correctness issue.
func (m *Model) enqueueDirectoryCopy(srcFS, dstFS fsys.FileSystem, srcRoot, dstRoot, label, destPane string) int {
	if err := dstFS.MkdirAll(dstRoot); err != nil {
		m.eventsCh <- transferQueueErrorMsg{label: label, err: err}
		return 0
	}

	entries, err := srcFS.ReadDir(srcRoot)
	if err != nil {
		m.eventsCh <- transferQueueErrorMsg{label: label, err: err}
		return 0
	}

	count := 0
	for _, e := range entries {
		srcPath := srcFS.Join(srcRoot, e.Name)
		dstPath := dstFS.Join(dstRoot, e.Name)
		childLabel := label + "/" + e.Name
		if e.IsDir {
			count += m.enqueueDirectoryCopy(srcFS, dstFS, srcPath, dstPath, childLabel, destPane)
			continue
		}
		m.enqueueFileCopy(srcFS, dstFS, srcPath, dstPath, e.Name, e.Size, destPane)
		count++
	}
	return count
}
