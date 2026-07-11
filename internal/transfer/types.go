package transfer

import "github.com/bvanhorn/exfil/internal/fsys"

type Direction string

const (
	DirectionDownload Direction = "download"
	DirectionUpload   Direction = "upload"
)

type Job struct {
	ID         int
	Direction  Direction
	SourcePath string
	DestPath   string
	Filename   string

	// SrcFS and DstFS carry the source/destination filesystems for this job.
	// This is how a worker knows whether either side is remote (RemoteFS) or
	// local (LocalFS) — the job is self-describing so the worker pool stays
	// generic. If either is nil, the worker falls back to LocalFS.
	SrcFS fsys.FileSystem
	DstFS fsys.FileSystem
}
