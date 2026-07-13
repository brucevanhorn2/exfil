package fsys

import (
	"io"
	"os"
	"time"
)

type Entry struct {
	Name    string
	Size    int64
	IsDir   bool
	Mode    os.FileMode
	ModTime time.Time
}

type FileSystem interface {
	ReadDir(path string) ([]Entry, error)
	Join(elem ...string) string
	Home() (string, error)
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
	Stat(path string) (*Entry, error)
	// Remove deletes a single file or an empty directory. Non-empty
	// directories are rejected by the underlying os/sftp call rather than
	// removed recursively — see issue #15 for recursive delete.
	Remove(path string) error
	Rename(oldPath, newPath string) error
	Mkdir(path string) error
}
