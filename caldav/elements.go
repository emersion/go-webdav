package caldav

import (
	"encoding/xml"

	"github.com/emersion/go-webdav/internal"
)

const namespace = "urn:ietf:params:xml:ns:caldav"

var calendarHomeSetName = xml.Name{namespace, "calendar-home-set"}

// https://tools.ietf.org/html/rfc4791#section-6.2.1
type calendarHomeSet struct {
	XMLName xml.Name      `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	Href    internal.Href `xml:"DAV: href"`
}
