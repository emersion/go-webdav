package internal

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// TODO: cache parsed value
type Status string

func NewStatus(code int, msg string) Status {
	if msg == "" {
		msg = http.StatusText(code)
	}
	return Status(fmt.Sprintf("HTTP/1.1 %v %v", code, msg))
}

func (s Status) parse() (int, string, error) {
	if s == "" {
		return http.StatusOK, "", nil
	}

	parts := strings.SplitN(string(s), " ", 3)
	if len(parts) != 3 {
		return 0, "", fmt.Errorf("webdav: invalid HTTP status %q: expected 3 fields", s)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", fmt.Errorf("webdav: invalid HTTP status %q: failed to parse code: %v", s, err)
	}
	msg := parts[2]

	return code, msg, nil
}

func (s Status) Err() error {
	code, msg, err := s.parse()
	if err != nil {
		return err
	}

	// TODO: handle 2xx, 3xx
	if code != http.StatusOK {
		return fmt.Errorf("webdav: HTTP error: %v %v", code, msg)
	}
	return nil
}

// https://tools.ietf.org/html/rfc4918#section-14.16
type Multistatus struct {
	XMLName             xml.Name   `xml:"DAV: multistatus"`
	Responses           []Response `xml:"response"`
	ResponseDescription string     `xml:"responsedescription,omitempty"`
}

func NewMultistatus(resps ...Response) *Multistatus {
	return &Multistatus{Responses: resps}
}

func (ms *Multistatus) Get(href string) (*Response, error) {
	for i := range ms.Responses {
		resp := &ms.Responses[i]
		for _, h := range resp.Hrefs {
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
	Hrefs               []string     `xml:"href"`
	Propstats           []Propstat   `xml:"propstat,omitempty"`
	ResponseDescription string       `xml:"responsedescription,omitempty"`
	Status              Status       `xml:"status,omitempty"`
	Error               *RawXMLValue `xml:"error,omitempty"`
	Location            *Location    `xml:"location,omitempty"`
}

func NewOKResponse(href string) *Response {
	return &Response{
		Hrefs:  []string{href},
		Status: NewStatus(http.StatusOK, ""),
	}
}

func (resp *Response) Href() (string, error) {
	if err := resp.Status.Err(); err != nil {
		return "", err
	}
	if len(resp.Hrefs) != 1 {
		return "", fmt.Errorf("webdav: malformed response: expected exactly one href element, got %v", len(resp.Hrefs))
	}
	return resp.Hrefs[0], nil
}

func (resp *Response) DecodeProp(v interface{}) error {
	name, err := valueXMLName(v)
	if err != nil {
		return err
	}
	if err := resp.Status.Err(); err != nil {
		return err
	}
	for i := range resp.Propstats {
		propstat := &resp.Propstats[i]
		for j := range propstat.Prop.Raw {
			raw := &propstat.Prop.Raw[j]
			if start, ok := raw.tok.(xml.StartElement); ok && name == start.Name {
				if err := propstat.Status.Err(); err != nil {
					return err
				}
				return raw.Decode(v)
			}
		}
	}

	return fmt.Errorf("webdav: missing prop %v %v in response", name.Space, name.Local)
}

func (resp *Response) EncodeProp(code int, v interface{}) error {
	raw, err := EncodeRawXMLElement(v)
	if err != nil {
		return err
	}

	for i := range resp.Propstats {
		propstat := &resp.Propstats[i]
		c, _, _ := propstat.Status.parse()
		if c == code {
			propstat.Prop.Raw = append(propstat.Prop.Raw, *raw)
			return nil
		}
	}

	resp.Propstats = append(resp.Propstats, Propstat{
		Status: NewStatus(code, ""),
		Prop:   Prop{Raw: []RawXMLValue{*raw}},
	})
	return nil
}

// https://tools.ietf.org/html/rfc4918#section-14.9
type Location struct {
	XMLName xml.Name `xml:"DAV: location"`
	Href    string   `xml:"href"`
}

// https://tools.ietf.org/html/rfc4918#section-14.22
type Propstat struct {
	XMLName             xml.Name     `xml:"DAV: propstat"`
	Prop                Prop         `xml:"prop"`
	Status              Status       `xml:"status"`
	ResponseDescription string       `xml:"responsedescription,omitempty"`
	Error               *RawXMLValue `xml:"error,omitempty"`
}

// https://tools.ietf.org/html/rfc4918#section-14.18
type Prop struct {
	XMLName xml.Name      `xml:"DAV: prop"`
	Raw     []RawXMLValue `xml:",any"`
}

func EncodeProp(values ...interface{}) (*Prop, error) {
	l := make([]RawXMLValue, len(values))
	for i, v := range values {
		raw, err := EncodeRawXMLElement(v)
		if err != nil {
			return nil, err
		}
		l[i] = *raw
	}
	return &Prop{Raw: l}, nil
}

func (prop *Prop) XMLNames() []xml.Name {
	l := make([]xml.Name, 0, len(prop.Raw))
	for _, raw := range prop.Raw {
		if start, ok := raw.tok.(xml.StartElement); ok {
			l = append(l, start.Name)
		}
	}
	return l
}

// https://tools.ietf.org/html/rfc4918#section-14.20
type Propfind struct {
	XMLName xml.Name `xml:"DAV: propfind"`
	Prop    *Prop    `xml:"prop,omitempty"`
	// TODO: propname | (allprop, include?)
}

func xmlNamesToRaw(names []xml.Name) []RawXMLValue {
	l := make([]RawXMLValue, len(names))
	for i, name := range names {
		l[i] = *NewRawXMLElement(name, nil, nil)
	}
	return l
}

func NewPropNamePropfind(names ...xml.Name) *Propfind {
	return &Propfind{Prop: &Prop{Raw: xmlNamesToRaw(names)}}
}

// https://tools.ietf.org/html/rfc4918#section-15.9
type ResourceType struct {
	XMLName xml.Name      `xml:"DAV: resourcetype"`
	Raw     []RawXMLValue `xml:",any"`
}

func NewResourceType(names ...xml.Name) *ResourceType {
	return &ResourceType{Raw: xmlNamesToRaw(names)}
}

func (t *ResourceType) Is(name xml.Name) bool {
	for _, raw := range t.Raw {
		if start, ok := raw.tok.(xml.StartElement); ok && name == start.Name {
			return true
		}
	}
	return false
}

var CollectionName = xml.Name{"DAV:", "collection"}
