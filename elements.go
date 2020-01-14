package webdav

import (
	"encoding/xml"
)

type currentUserPrincipalProp struct {
	Name xml.Name `xml:"DAV: current-user-principal"`
	Href string   `xml:"href"`
}
