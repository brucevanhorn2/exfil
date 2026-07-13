package fsys

import (
	"io"
	"path"

	"github.com/pkg/sftp"
)

type RemoteFS struct {
	client *sftp.Client
}

func NewRemoteFS(client *sftp.Client) *RemoteFS {
	return &RemoteFS{client: client}
}

func (rfs *RemoteFS) ReadDir(path string) ([]Entry, error) {
	entries, err := rfs.client.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]Entry, 0, len(entries))
	for _, info := range entries {
		result = append(result, Entry{
			Name:    info.Name(),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

func (rfs *RemoteFS) Join(elem ...string) string {
	return path.Join(elem...)
}

func (rfs *RemoteFS) Home() (string, error) {
	wd, err := rfs.client.Getwd()
	if err != nil {
		return "/", nil
	}
	return wd, nil
}

func (rfs *RemoteFS) Open(path string) (io.ReadCloser, error) {
	return rfs.client.Open(path)
}

func (rfs *RemoteFS) Create(path string) (io.WriteCloser, error) {
	return rfs.client.Create(path)
}

func (rfs *RemoteFS) Remove(path string) error {
	return rfs.client.Remove(path)
}

func (rfs *RemoteFS) Rename(oldPath, newPath string) error {
	return rfs.client.Rename(oldPath, newPath)
}

func (rfs *RemoteFS) Mkdir(path string) error {
	return rfs.client.Mkdir(path)
}

func (rfs *RemoteFS) Stat(path string) (*Entry, error) {
	info, err := rfs.client.Stat(path)
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
