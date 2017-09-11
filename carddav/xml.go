package carddav

import (
	"encoding/xml"

	"github.com/emersion/go-webdav"
)

// https://tools.ietf.org/html/rfc6352#section-10.7
type addressbookMultiget struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-multiget"`
	Allprop  *struct{}     `xml:"DAV: allprop"`
	Propname *struct{}     `xml:"DAV: propname"`
	Prop     webdav.PropfindProps `xml:"DAV: prop"`
	Href      []string   `xml:"DAV: href"`
}

// TODO
type addressData struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav address-data"`
	ContentType string `xml:"content-type,attr"`
	Version string `xml:"version,attr"`
	Prop []string `xml:"prop>name,attr"`
}
