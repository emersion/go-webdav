package webdav

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type LocalFileSystem string

func (fs LocalFileSystem) path(name string) (string, error) {
	if (filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0) || strings.Contains(name, "\x00") {
		return "", HTTPErrorf(http.StatusBadRequest, "webdav: invalid character in path")
	}
	name = path.Clean(name)
	if !path.IsAbs(name) {
		return "", HTTPErrorf(http.StatusBadRequest, "webdav: expected absolute path")
	}
	return filepath.Join(string(fs), filepath.FromSlash(name)), nil
}

func (fs LocalFileSystem) Open(name string) (File, error) {
	p, err := fs.path(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

var _ FileSystem = LocalFileSystem("")
