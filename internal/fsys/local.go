package fsys

import (
	"io"
	"os"
	"path/filepath"
)

type LocalFS struct{}

func (lfs LocalFS) ReadDir(path string) ([]Entry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, Entry{
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

func (lfs LocalFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}

func (lfs LocalFS) Home() (string, error) {
	return os.UserHomeDir()
}

func (lfs LocalFS) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (lfs LocalFS) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

func (lfs LocalFS) Remove(path string) error {
	return os.Remove(path)
}

// RemoveAll silently succeeds if path is already gone (os.RemoveAll's
// documented behavior) — unlike RemoteFS.RemoveAll, which Stats first and
// errors on a missing path. In deleteCmd's stop-at-first-error loop over
// multiple targets, that asymmetry only matters if something external
// deletes a marked target between marking and confirming.
func (lfs LocalFS) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (lfs LocalFS) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (lfs LocalFS) Mkdir(path string) error {
	return os.Mkdir(path, 0755)
}

func (lfs LocalFS) MkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func (lfs LocalFS) Stat(path string) (*Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &Entry{
		Name:    info.Name(),
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
	}, nil
}
