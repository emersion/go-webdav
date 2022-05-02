// Package webdav provides a client and server WebDAV filesystem implementation.
//
// WebDAV is defined in RFC 4918.
package webdav

import (
	"io/fs"
	"path"
	"time"
)

type FileInfo struct {
	Path     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
	MIMEType string
	ETag     string
}

type fileInfo struct {
	FileInfo
}

var (
	_ fs.FileInfo = (*fileInfo)(nil)
	_ fs.DirEntry = (*fileInfo)(nil)
)

func (fi *fileInfo) Name() string {
	return path.Base(fi.Path)
}

func (fi *fileInfo) Size() int64 {
	return fi.FileInfo.Size
}

func (fi *fileInfo) Mode() fs.FileMode {
	var mode fs.FileMode
	if fi.FileInfo.IsDir {
		mode |= fs.ModeDir
	}
	return mode
}

func (fi *fileInfo) ModTime() time.Time {
	return fi.FileInfo.ModTime
}

func (fi *fileInfo) IsDir() bool {
	return fi.FileInfo.IsDir
}

func (fi *fileInfo) Sys() interface{} {
	return nil
}

func (fi *fileInfo) Type() fs.FileMode {
	return fi.Mode()
}

func (fi *fileInfo) Info() (fs.FileInfo, error) {
	return fi, nil
}
