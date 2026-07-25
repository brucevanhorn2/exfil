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
	group    *dirCopyGroup
	srcPane  string
	destPane string
}

// transferQueueErrorMsg reports a failure encountered while walking a
// directory (MkdirAll or ReadDir) — surfaced via m.statusMsg since it
// happens before any transfer.Job (and therefore any queue pane row) exists
// for it to attach to. group (non-nil for a subtree failure inside a
// recursive directory copy) lets Update() mark that directory's group as
// failed, so its selection mark is left in place once the walk finishes.
type transferQueueErrorMsg struct {
	label string
	err   error
	group *dirCopyGroup
}

// dirWalkDoneMsg reports that the recursive walk for one top-level marked
// directory (identified by group) has fully finished discovering and
// enqueueing its files. Sent once, from enqueueCopyDirection's returned
// Cmd, strictly after enqueueDirectoryCopy's top-level call returns — so it
// always arrives after every transferQueuedMsg/transferQueueErrorMsg that
// walk produced for the same group (same goroutine, same channel, FIFO).
// discovered is the total file count that walk found (possibly 0, for an
// empty directory or one that failed to walk at all).
type dirWalkDoneMsg struct {
	group      *dirCopyGroup
	discovered int
}

// enqueueFileCopy allocates a transfer ID and hands the job to the worker
// pool. Safe to call from any goroutine: it only sends on m.eventsCh/
// m.jobsCh and calls the mutex-guarded allocateTransferID, never touching
// m.queuePane/m.statusMsg/m.nextID directly. group is nil for a flat
// (non-directory) copy.
func (m *Model) enqueueFileCopy(srcFS, dstFS fsys.FileSystem, srcPath, dstPath, filename string, size int64, group *dirCopyGroup, srcPane, destPane string) {
	id := m.allocateTransferID()
	m.eventsCh <- transferQueuedMsg{id: id, filename: filename, total: size, group: group, srcPane: srcPane, destPane: destPane}
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
func (m *Model) enqueueDirectoryCopy(srcFS, dstFS fsys.FileSystem, srcRoot, dstRoot, label string, group *dirCopyGroup, srcPane, destPane string) int {
	if err := dstFS.MkdirAll(dstRoot); err != nil {
		m.eventsCh <- transferQueueErrorMsg{label: label, err: err, group: group}
		return 0
	}

	entries, err := srcFS.ReadDir(srcRoot)
	if err != nil {
		m.eventsCh <- transferQueueErrorMsg{label: label, err: err, group: group}
		return 0
	}

	count := 0
	for _, e := range entries {
		srcPath := srcFS.Join(srcRoot, e.Name)
		dstPath := dstFS.Join(dstRoot, e.Name)
		childLabel := label + "/" + e.Name
		if e.IsDir {
			count += m.enqueueDirectoryCopy(srcFS, dstFS, srcPath, dstPath, childLabel, group, srcPane, destPane)
			continue
		}
		m.enqueueFileCopy(srcFS, dstFS, srcPath, dstPath, e.Name, e.Size, group, srcPane, destPane)
		count++
	}
	return count
}
