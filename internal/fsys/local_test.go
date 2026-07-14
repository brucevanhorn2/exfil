package fsys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFSRemove(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(file, []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := (LocalFS{}).Remove(file); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected file to be gone, stat err = %v", err)
	}
}

func TestLocalFSRemoveNonEmptyDirFails(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := (LocalFS{}).Remove(sub); err == nil {
		t.Fatal("expected Remove to fail on a non-empty directory (issue #4 keeps delete non-recursive)")
	}
}

func TestLocalFSRename(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(oldPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := (LocalFS{}).Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be gone, stat err = %v", err)
	}
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Errorf("got %q, want %q", got, "content")
	}
}

func TestLocalFSMkdir(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "child")

	if err := (LocalFS{}).Mkdir(newDir); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", newDir)
	}
}
