package webdav

import (
	"encoding/xml"
)

type currentUserPrincipal struct {
	XMLName xml.Name `xml:"DAV: current-user-principal"`
	Href    string   `xml:"href"`
}
