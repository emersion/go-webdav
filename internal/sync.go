package internal

import (
	"encoding/xml"
	"net/http"
)

// Collection synchronization (RFC 6578) on the server side, and the
// calendarserver.org getctag clients poll before they sync.

var (
	SyncTokenName          = xml.Name{Namespace, "sync-token"}
	SyncCollectionName     = xml.Name{Namespace, "sync-collection"}
	SupportedReportSetName = xml.Name{Namespace, "supported-report-set"}
	GetCTagName            = xml.Name{"http://calendarserver.org/ns/", "getctag"}
)

// https://tools.ietf.org/html/rfc6578#section-4
type SyncToken struct {
	XMLName xml.Name `xml:"DAV: sync-token"`
	Token   string   `xml:",chardata"`
}

type GetCTag struct {
	XMLName xml.Name `xml:"http://calendarserver.org/ns/ getctag"`
	CTag    string   `xml:",chardata"`
}

// https://tools.ietf.org/html/rfc3253#section-3.1.5
type SupportedReportSet struct {
	XMLName         xml.Name          `xml:"DAV: supported-report-set"`
	SupportedReport []SupportedReport `xml:"supported-report"`
}

type SupportedReport struct {
	Report ReportName `xml:"report"`
}

type ReportName struct {
	Raw RawXMLValue `xml:",any"`
}

// NewSupportedReportSet lists the REPORT names a collection answers.
func NewSupportedReportSet(names ...xml.Name) *SupportedReportSet {
	set := &SupportedReportSet{}
	for _, name := range names {
		set.SupportedReport = append(set.SupportedReport, SupportedReport{
			Report: ReportName{Raw: *NewRawXMLElement(name, nil, nil)},
		})
	}
	return set
}

// NewSyncTokenError is the answer to a sync-collection whose token the
// server does not know (RFC 6578 section 3.2): the client syncs anew.
func NewSyncTokenError() error {
	elem := NewRawXMLElement(xml.Name{Namespace, "valid-sync-token"}, nil, nil)
	return &HTTPError{
		Code: http.StatusForbidden,
		Err:  &Error{Raw: []RawXMLValue{*elem}},
	}
}

// NewNotFoundResponse is a sync-collection member that is gone.
func NewNotFoundResponse(path string) *Response {
	return &Response{
		Hrefs:  []Href{{Path: path}},
		Status: &Status{Code: http.StatusNotFound},
	}
}
