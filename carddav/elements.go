package carddav

import (
	"encoding/xml"
	"fmt"

	"github.com/emersion/go-webdav/internal"
)

const namespace = "urn:ietf:params:xml:ns:carddav"

var (
	addressBookName                 = xml.Name{namespace, "addressbook"}
	addressBookHomeSetName          = xml.Name{namespace, "addressbook-home-set"}
	addressBookDescriptionName      = xml.Name{namespace, "addressbook-description"}
	addressBookQueryName            = xml.Name{namespace, "addressbook-query"}
	addressBookMultigetName         = xml.Name{namespace, "addressbook-multiget"}
	addressBookSupportedAddressData = xml.Name{namespace, "addressbook-supported-address-data"}

	addressDataName = xml.Name{namespace, "address-data"}

	maxResourceSizeName = xml.Name{namespace, "max-resource-size"}
)

type addressbookHomeSet struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-home-set"`
	Href    string   `xml:"href"`
}

type addressbookDescription struct {
	XMLName     xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-description"`
	Description string   `xml:",chardata"`
}

// https://tools.ietf.org/html/rfc6352#section-6.2.2
type addressbookSupportedAddressData struct {
	XMLName xml.Name          `xml:"urn:ietf:params:xml:ns:carddav addressbook-supported-address-data"`
	Types   []addressDataType `xml:"address-data-type"`
}

type addressDataType struct {
	XMLName     xml.Name `xml:"urn:ietf:params:xml:ns:carddav address-data-type"`
	ContentType string   `xml:"content-type,attr"`
	Version     string   `xml:"version,attr"`
}

// https://tools.ietf.org/html/rfc6352#section-6.2.3
type maxResourceSize struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:carddav max-resource-size"`
	Size    int64    `xml:",chardata"`
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

type reportReq struct {
	Query    *addressbookQuery
	Multiget *addressbookMultiget
}

func (r *reportReq) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var v interface{}
	switch start.Name {
	case addressBookQueryName:
		r.Query = &addressbookQuery{}
		v = r.Query
	case addressBookMultigetName:
		r.Multiget = &addressbookMultiget{}
		v = r.Multiget
	default:
		return fmt.Errorf("carddav: unsupported REPORT root %q %q", start.Name.Space, start.Name.Local)
	}

	return d.DecodeElement(v, &start)
}
