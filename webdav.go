// Package webdav provides a client and server WebDAV filesystem implementation.
//
// WebDAV is defined in RFC 4918.
package webdav

import (
	"time"
)

// Principal is a DAV principal as defined in RFC 3744 section 2.
type Principal struct {
	Path string
	Name string
}

type FileInfo struct {
	Path     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
	MIMEType string
	ETag     string
}
