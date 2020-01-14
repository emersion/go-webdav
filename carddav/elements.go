package carddav

import (
	"encoding/xml"
)

type addressbookHomeSet struct {
	Name xml.Name `xml:"urn:ietf:params:xml:ns:carddav addressbook-home-set"`
	Href string   `xml:"href"`
}
