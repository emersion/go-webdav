package webdav

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFileSystemCopyDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"src/a.txt": "a", "src/sub/b.txt": "b", "dst/old.txt": "old"} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fs := LocalFileSystem(dir)
	if _, err := fs.Copy(context.Background(), "/src", "/dst", &CopyOptions{}); err != nil {
		t.Fatalf("Copy() = %v", err)
	}

	for name, want := range map[string]string{"dst/a.txt": "a", "dst/sub/b.txt": "b"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "dst/old.txt")); !os.IsNotExist(err) {
		t.Errorf("dst/old.txt still exists after overwrite: %v", err)
	}
}

func TestLocalFileSystemMoveMissingSource(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(dst, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := LocalFileSystem(dir)
	if _, err := fs.Move(context.Background(), "/draft.txt", "/report.txt", &MoveOptions{}); err == nil {
		t.Fatal("Move() of a missing source succeeded")
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "v1" {
		t.Errorf("report.txt = %q, %v; want %q", got, err, "v1")
	}
}

type failingReader struct {
	read int
}

func (r *failingReader) Read(p []byte) (int, error) {
	r.read++
	if r.read > 1 {
		return 0, errors.New("interrupted body")
	}
	return copy(p, []byte("partial")), nil
}

func (r *failingReader) Close() error { return nil }

func TestLocalFileSystemCreateInterruptedOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(p, []byte("important original contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := LocalFileSystem(dir)
	if _, _, err := fs.Create(context.Background(), "/notes.txt", &failingReader{}, &CreateOptions{}); err == nil {
		t.Fatal("Create() with interrupted body succeeded")
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("original file was removed: %v", err)
	}
	if string(got) != "important original contents" {
		t.Errorf("notes.txt = %q; want %q", got, "important original contents")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}
