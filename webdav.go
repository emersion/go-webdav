// Package webdav provides a client and server WebDAV filesystem implementation.
//
// WebDAV is defined in RFC 4918.
package webdav

import (
	"time"
)

// TODO: add ETag, MIMEType to FileInfo

type FileInfo struct {
	Href    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}
