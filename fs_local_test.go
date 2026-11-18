package webdav

import (
	"context"
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
