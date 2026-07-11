package transfer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bvanhorn/exfil/internal/fsys"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRunWithFSLocalCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	want := []byte("hello exfil, this is a test payload\n")
	if err := os.WriteFile(src, want, 0644); err != nil {
		t.Fatal(err)
	}

	events := make(chan tea.Msg, 64)
	go RunWithFS(Job{ID: 1, SourcePath: src, DestPath: dst, Filename: "src.txt"},
		events, fsys.LocalFS{}, fsys.LocalFS{})

	var gotProgress, gotDone bool
	for msg := range events {
		switch m := msg.(type) {
		case TransferProgressMsg:
			gotProgress = true
		case TransferDoneMsg:
			gotDone = true
			close(events)
		case TransferErrorMsg:
			t.Fatalf("unexpected error: %v", m.Err)
		default:
			t.Fatalf("unexpected message type %T (orphan message would kill the UI subscription)", m)
		}
	}

	if !gotProgress {
		t.Error("expected at least one progress message")
	}
	if !gotDone {
		t.Error("expected a done message")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("copied content mismatch:\n got %q\nwant %q", got, want)
	}
}
