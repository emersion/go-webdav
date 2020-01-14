package webdav

import (
	"encoding/xml"
)

type currentUserPrincipal struct {
	Name xml.Name `xml:"DAV: current-user-principal"`
	Href string   `xml:"href"`
}
