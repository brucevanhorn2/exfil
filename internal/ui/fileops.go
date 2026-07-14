package ui

import (
	"errors"

	"github.com/bvanhorn/exfil/internal/fsys"
	tea "github.com/charmbracelet/bubbletea"
)

// errRenameTargetExists is a sentinel fileOpDoneMsg.err value, checked via
// errors.Is in Update() to show a dedicated (localized) message rather than
// the generic "rename failed: %v" — os.Rename silently overwrites an
// existing destination on LocalFS, so this check has to happen before the
// call, not be inferred from its error.
var errRenameTargetExists = errors.New("rename target already exists")

// fileOpDoneMsg reports the outcome of a delete/rename/mkdir op. pane names
// which BrowserPane ("local"/"remote") to refresh, matching the
// TransferDoneMsg/transferDest convention used for copies.
type fileOpDoneMsg struct {
	pane   string
	action string // "delete", "rename", "mkdir"
	err    error
}

// deleteTarget names one entry marked for deletion and whether it's a
// directory — decides Remove (file or empty dir) vs RemoveAll (recursive)
// per entry, so one delete operation can mix plain files and non-empty
// directories (issue #15).
type deleteTarget struct {
	Name  string
	IsDir bool
}

// deleteCmd removes each target from cwd, off the UI thread since
// RemoteFS.Remove/RemoveAll is a network round-trip. It stops at the first
// error rather than continuing, so the reported error is unambiguous.
func deleteCmd(fs fsys.FileSystem, cwd string, targets []deleteTarget, pane string) tea.Cmd {
	return func() tea.Msg {
		for _, t := range targets {
			path := fs.Join(cwd, t.Name)
			var err error
			if t.IsDir {
				err = fs.RemoveAll(path)
			} else {
				err = fs.Remove(path)
			}
			if err != nil {
				return fileOpDoneMsg{pane: pane, action: "delete", err: err}
			}
		}
		return fileOpDoneMsg{pane: pane, action: "delete"}
	}
}

// renameCmd refuses to rename onto an existing entry: os.Rename (LocalFS)
// silently overwrites the destination with no confirmation, which would be
// inconsistent with delete's explicit Y/N screen in the same feature — and
// asymmetric with RemoteFS, where sftp's plain (non-Posix) Rename already
// rejects an existing destination per the SFTP spec.
func renameCmd(fs fsys.FileSystem, cwd, oldName, newName, pane string) tea.Cmd {
	return func() tea.Msg {
		newPath := fs.Join(cwd, newName)
		if _, err := fs.Stat(newPath); err == nil {
			return fileOpDoneMsg{pane: pane, action: "rename", err: errRenameTargetExists}
		}
		err := fs.Rename(fs.Join(cwd, oldName), newPath)
		return fileOpDoneMsg{pane: pane, action: "rename", err: err}
	}
}

func mkdirCmd(fs fsys.FileSystem, cwd, name, pane string) tea.Cmd {
	return func() tea.Msg {
		err := fs.Mkdir(fs.Join(cwd, name))
		return fileOpDoneMsg{pane: pane, action: "mkdir", err: err}
	}
}

func fileOpErrorKey(action string) string {
	switch action {
	case "rename":
		return "status_rename_error"
	case "mkdir":
		return "status_mkdir_error"
	default:
		return "status_delete_error"
	}
}

func fileOpSuccessKey(action string) string {
	switch action {
	case "rename":
		return "status_rename_success"
	case "mkdir":
		return "status_mkdir_success"
	default:
		return "status_delete_success"
	}
}
