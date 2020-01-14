package carddav

import (
	"encoding/xml"
)

type addressbookHomeSet struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-home-set"`
	Href    string   `xml:"href"`
}

type addressbookDescription struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-description"`
	Data    string   `xml:",chardata"`
}

// https://tools.ietf.org/html/rfc6352#section-10.3
type addressbookQuery struct {
	XMLName xml.Name       `xml:"urn:ietf:params:xml:ns:carddav addressbook-query"`
	Prop    *internal.Prop `xml:"DAV: prop,omitempty"`
	// TODO: DAV:allprop | DAV:propname
	// TODO: filter, limit?
}
