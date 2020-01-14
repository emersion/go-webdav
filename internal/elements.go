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

// https://tools.ietf.org/html/rfc4918#section-14.20
type Propfind struct {
	XMLName xml.Name     `xml:"DAV: propfind"`
	Prop    *RawXMLValue `xml:"DAV: prop,omitempty"`
	// TODO: propname | (allprop, include?)
}

func NewPropPropfind(names ...xml.Name) *Propfind {
	children := make([]RawXMLValue, len(names))
	for i, name := range names {
		children[i] = *NewRawXMLElement(name, nil, nil)
	}
	prop := NewRawXMLElement(xml.Name{"DAV:", "prop"}, nil, children)
	return &Propfind{Prop: prop}
}
