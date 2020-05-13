// Package caldav provides a client and server CalDAV implementation.
//
// CalDAV is defined in RFC 4791.
package caldav

import (
	"time"

	"github.com/emersion/go-ical"
)

type Calendar struct {
	Path            string
	Name            string
	Description     string
	MaxResourceSize int64
}

type CalendarCompRequest struct {
	Name string

	AllProps bool
	Props    []string

	AllComps bool
	Comps    []CalendarCompRequest
}

type CompFilter struct {
	Name       string
	Start, End time.Time
	Props      []PropFilter
	Comps      []CompFilter
}

type PropFilter struct {
	Name      string
	TextMatch *TextMatch
}

type TextMatch struct {
	Text string
}

type CalendarQuery struct {
	CompRequest CalendarCompRequest
	CompFilter  CompFilter
}

type CalendarMultiGet struct {
	Paths       []string
	CompRequest CalendarCompRequest
}

type CalendarObject struct {
	Path    string
	ModTime time.Time
	ETag    string
	Data    *ical.Calendar
}
