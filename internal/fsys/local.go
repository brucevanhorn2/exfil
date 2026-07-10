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
