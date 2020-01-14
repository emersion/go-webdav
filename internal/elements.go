package internal

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Status string

func (s Status) Err() error {
	parts := strings.SplitN(string(s), " ", 3)
	if len(parts) != 3 {
		return fmt.Errorf("webdav: invalid HTTP status %q: expected 3 fields", s)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("webdav: invalid HTTP status %q: failed to parse code: %v", s, err)
	}
	msg := parts[2]

	// TODO: handle 2xx, 3xx
	if code != http.StatusOK {
		return fmt.Errorf("webdav: HTTP error: %v %v", code, msg)
	}
	return nil
}

// https://tools.ietf.org/html/rfc4918#section-14.16
type Multistatus struct {
	XMLName             xml.Name   `xml:"DAV: multistatus"`
	Responses           []Response `xml:"DAV: response"`
	ResponseDescription string     `xml:"DAV: responsedescription,omitempty"`
}

func (ms *Multistatus) Get(href string) (*Response, error) {
	for i := range ms.Responses {
		resp := &ms.Responses[i]
		for _, h := range resp.Href {
			if h == href {
				return resp, nil
			}
		}
	}

	return nil, fmt.Errorf("webdav: missing response for href %q", href)
}

// https://tools.ietf.org/html/rfc4918#section-14.24
type Response struct {
	XMLName             xml.Name     `xml:"DAV: response"`
	Href                []string     `xml:"DAV: href"`
	Propstats           []Propstat   `xml:"DAV: propstat,omitempty"`
	ResponseDescription string       `xml:"DAV: responsedescription,omitempty"`
	Status              Status       `xml:"DAV: status,omitempty"`
	Error               *RawXMLValue `xml:"DAV: error,omitempty"`
	Location            *Location    `xml:"DAV: location,omitempty"`
}

func (resp *Response) DecodeProp(name xml.Name, v interface{}) error {
	for i := range resp.Propstats {
		propstat := &resp.Propstats[i]
		for j := range propstat.Prop.Raw {
			raw := &propstat.Prop.Raw[j]
			if start, ok := raw.tok.(xml.StartElement); ok {
				if name == start.Name {
					if err := propstat.Status.Err(); err != nil {
						return err
					}
					return raw.Decode(v)
				}
			}
		}
	}

	return fmt.Errorf("webdav: missing prop %v %v in response", name.Space, name.Local)
}

// https://tools.ietf.org/html/rfc4918#section-14.9
type Location struct {
	XMLName xml.Name `xml:"DAV: location"`
	Href    string   `xml:"DAV: href"`
}

// https://tools.ietf.org/html/rfc4918#section-14.22
type Propstat struct {
	XMLName             xml.Name     `xml:"DAV: propstat"`
	Prop                Prop         `xml:"DAV: prop"`
	Status              Status       `xml:"DAV: status"`
	ResponseDescription string       `xml:"DAV: responsedescription,omitempty"`
	Error               *RawXMLValue `xml:"DAV: error,omitempty"`
}

// https://tools.ietf.org/html/rfc4918#section-14.18
type Prop struct {
	XMLName xml.Name      `xml:"DAV: prop"`
	Raw     []RawXMLValue `xml:",any"`
}

// https://tools.ietf.org/html/rfc4918#section-14.20
type Propfind struct {
	XMLName xml.Name `xml:"DAV: propfind"`
	Prop    *Prop    `xml:"DAV: prop,omitempty"`
	// TODO: propname | (allprop, include?)
}

func NewPropPropfind(names ...xml.Name) *Propfind {
	children := make([]RawXMLValue, len(names))
	for i, name := range names {
		children[i] = *NewRawXMLElement(name, nil, nil)
	}
	return &Propfind{Prop: &Prop{Raw: children}}
}
