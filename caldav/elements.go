package caldav

import (
	"encoding/xml"

	"github.com/emersion/go-webdav/internal"
)

const namespace = "urn:ietf:params:xml:ns:caldav"

var (
	calendarHomeSetName = xml.Name{namespace, "calendar-home-set"}

	calendarDescriptionName   = xml.Name{namespace, "calendar-description"}
	supportedCalendarDataName = xml.Name{namespace, "supported-calendar-data"}
	maxResourceSizeName       = xml.Name{namespace, "max-resource-size"}

	calendarName = xml.Name{namespace, "calendar"}
)

// https://tools.ietf.org/html/rfc4791#section-6.2.1
type calendarHomeSet struct {
	XMLName xml.Name      `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	Href    internal.Href `xml:"DAV: href"`
}

// https://tools.ietf.org/html/rfc4791#section-5.2.1
type calendarDescription struct {
	XMLName     xml.Name `xml:"urn:ietf:params:xml:ns:caldav calendar-description"`
	Description string   `xml:",chardata"`
}

// https://tools.ietf.org/html/rfc4791#section-5.2.4
type supportedCalendarData struct {
	XMLName xml.Name           `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-data"`
	Types   []calendarDataType `xml:"calendar-data"`
}

// https://tools.ietf.org/html/rfc4791#section-9.6
type calendarDataType struct {
	XMLName     xml.Name `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	ContentType string   `xml:"content-type,attr"`
	Version     string   `xml:"version,attr"`
}

// https://tools.ietf.org/html/rfc4791#section-5.2.5
type maxResourceSize struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav max-resource-size"`
	Size    int64    `xml:",chardata"`
}

// https://tools.ietf.org/html/rfc4791#section-9.5
type calendarQuery struct {
	XMLName  xml.Name       `xml:"urn:ietf:params:xml:ns:caldav calendar-query"`
	Prop     *internal.Prop `xml:"DAV: prop,omitempty"`
	AllProp  *struct{}      `xml:"DAV: allprop,omitempty"`
	PropName *struct{}      `xml:"DAV: propname,omitempty"`
	// TODO: filter, timezone
}

// Request variant of https://tools.ietf.org/html/rfc4791#section-9.6
type calendarDataReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	Comp    *comp    `xml:"comp,omitempty"`
	// TODO: expand, limit-recurrence-set, limit-freebusy-set
}

// https://tools.ietf.org/html/rfc4791#section-9.6.1
type comp struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav comp"`
	Name    string   `xml:"name,attr"`

	Allprop *struct{} `xml:"allprop,omitempty"`
	Prop    []prop    `xml:"prop,omitempty"`

	Allcomp *struct{} `xml:"allcomp,omitempty"`
	Comp    []comp    `xml:"comp,omitempty"`
}

// https://tools.ietf.org/html/rfc4791#section-9.6.4
type prop struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav prop"`
	Name    string   `xml:"name,attr"`
	// TODO: novalue
}

// Response variant of https://tools.ietf.org/html/rfc4791#section-9.6
type calendarData struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	Data    []byte   `xml:",chardata"`
}
