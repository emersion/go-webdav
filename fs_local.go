package webdav

import (
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/emersion/go-webdav/internal"
)

type LocalFileSystem string

func (fs LocalFileSystem) path(name string) (string, error) {
	if (filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0) || strings.Contains(name, "\x00") {
		return "", internal.HTTPErrorf(http.StatusBadRequest, "webdav: invalid character in path")
	}
	name = path.Clean(name)
	if !path.IsAbs(name) {
		return "", internal.HTTPErrorf(http.StatusBadRequest, "webdav: expected absolute path")
	}
	return filepath.Join(string(fs), filepath.FromSlash(name)), nil
}

func (fs LocalFileSystem) Open(name string) (io.ReadCloser, error) {
	p, err := fs.path(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (fs LocalFileSystem) Stat(name string) (os.FileInfo, error) {
	p, err := fs.path(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (fs LocalFileSystem) Readdir(name string) ([]os.FileInfo, error) {
	p, err := fs.path(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdir(-1)
}

func (fs LocalFileSystem) Create(name string) (io.WriteCloser, error) {
	p, err := fs.path(name)
	if err != nil {
		return nil, err
	}
	return os.Create(p)
}

func (fs LocalFileSystem) RemoveAll(name string) error {
	p, err := fs.path(name)
	if err != nil {
		return err
	}

	// WebDAV semantics are that it should return a "404 Not Found" error in
	// case the resource doesn't exist. We need to Stat before RemoveAll.
	if _, err = os.Stat(p); err != nil {
		return err
	}

	return os.RemoveAll(p)
}

func (fs LocalFileSystem) Mkdir(name string) error {
	p, err := fs.path(name)
	if err != nil {
		return err
	}
	return os.Mkdir(p, 0755)
}

var _ FileSystem = LocalFileSystem("")
