// Package webdav provides a client and server WebDAV filesystem implementation.
//
// WebDAV is defined in RFC 4918.
package webdav

import (
	"time"
)

// FileInfo holds information about a WebDAV file.
type FileInfo struct {
	Path     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
	MIMEType string
	ETag     string
}

type CopyOptions struct {
	NoRecursive bool
	NoOverwrite bool
}
