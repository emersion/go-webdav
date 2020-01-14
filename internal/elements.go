package internal

import (
	"encoding/xml"
)

// https://tools.ietf.org/html/rfc4918#section-14.16
type multistatus struct {
	XMLName   xml.Name   `xml:"DAV: multistatus"`
	Responses []Response `xml:"DAV: response"`
	// TODO: responsedescription?
}

// https://tools.ietf.org/html/rfc4918#section-14.24
type Response struct {
	XMLName   xml.Name   `xml:"DAV: response"`
	Href      string     `xml:"DAV: href"`
	Propstats []Propstat `xml:"DAV: propstat"`
	// TODO: (href*, status)
	// TODO: error?, responsedescription? , location?
}

// https://tools.ietf.org/html/rfc4918#section-14.22
type Propstat struct {
	XMLName xml.Name    `xml:"DAV: propstat"`
	Prop    RawXMLValue `xml:"DAV: prop"`
	Status  string      `xml:"DAV: status"`
	// TODO: error?, responsedescription?
}
