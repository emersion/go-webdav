// Package caldav provides a client and server CalDAV implementation.
//
// CalDAV is defined in RFC 4791.
package caldav

type Calendar struct {
	Path            string
	Name            string
	Description     string
	MaxResourceSize int64
}
