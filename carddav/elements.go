package carddav

import (
	"encoding/xml"

	"github.com/emersion/go-webdav/internal"
)

const namespace = "urn:ietf:params:xml:ns:carddav"

var addressBookName = xml.Name{namespace, "addressbook"}

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

// https://tools.ietf.org/html/rfc6352#section-8.7
type addressbookMultiget struct {
	XMLName xml.Name       `xml:"urn:ietf:params:xml:ns:carddav addressbook-multiget"`
	Hrefs   []string       `xml:"DAV: href"`
	Prop    *internal.Prop `xml:"DAV: prop,omitempty"`
	// TODO: DAV:allprop | DAV:propname
}

func newProp(name string, noValue bool) *internal.RawXMLValue {
	attrs := []xml.Attr{{Name: xml.Name{namespace, "name"}, Value: name}}
	if noValue {
		attrs = append(attrs, xml.Attr{Name: xml.Name{namespace, "novalue"}, Value: "yes"})
	}

	xmlName := xml.Name{namespace, "prop"}
	return internal.NewRawXMLElement(xmlName, attrs, nil)
}

// https://tools.ietf.org/html/rfc6352#section-10.4
type addressDataReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav address-data"`
	Props   []prop   `xml:"prop"`
	// TODO: allprop
}

// https://tools.ietf.org/html/rfc6352#section-10.4.2
type prop struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav prop"`
	Name    string   `xml:"name,attr"`
	// TODO: novalue
}

// https://tools.ietf.org/html/rfc6352#section-10.4
type addressDataResp struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav address-data"`
	Data    []byte   `xml:",chardata"`
}
