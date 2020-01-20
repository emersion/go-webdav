package internal

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
)

// Depth indicates whether a request applies to the resource's members. It's
// defined in RFC 4918 section 10.2.
type Depth int

const (
	// DepthZero indicates that the request applies only to the resource.
	DepthZero Depth = 0
	// DepthOne indicates that the request applies to the resource and its
	// internal members only.
	DepthOne Depth = 1
	// DepthInfinity indicates that the request applies to the resource and all
	// of its members.
	DepthInfinity Depth = -1
)

// ParseDepth parses a Depth header.
func ParseDepth(s string) (Depth, error) {
	switch s {
	case "0":
		return DepthZero, nil
	case "1":
		return DepthOne, nil
	case "infinity":
		return DepthInfinity, nil
	}
	return 0, fmt.Errorf("webdav: invalid Depth value")
}

// String formats the depth.
func (d Depth) String() string {
	switch d {
	case DepthZero:
		return "0"
	case DepthOne:
		return "1"
	case DepthInfinity:
		return "infinity"
	}
	panic("webdav: invalid Depth value")
}

type Client struct {
	http               *http.Client
	endpoint           *url.URL
	username, password string
}

func NewClient(c *http.Client, endpoint string) (*Client, error) {
	if c == nil {
		c = http.DefaultClient
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{http: c, endpoint: u}, nil
}

func (c *Client) SetBasicAuth(username, password string) {
	c.username = username
	c.password = password
}

func (c *Client) NewRequest(method string, href string, body io.Reader) (*http.Request, error) {
	hrefURL, err := url.Parse(href)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request href %q: %v", href, err)
	}

	u := url.URL{
		Scheme:   c.endpoint.Scheme,
		User:     c.endpoint.User,
		Host:     c.endpoint.Host,
		Path:     path.Join(c.endpoint.Path, hrefURL.Path),
		RawQuery: hrefURL.RawQuery,
	}
	return http.NewRequest(method, u.String(), body)
}

func (c *Client) NewXMLRequest(method string, href string, v interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	if err := xml.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}

	req, err := c.NewRequest(method, href, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")

	return req, nil
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return c.http.Do(req)
}

func (c *Client) DoMultiStatus(req *http.Request) (*Multistatus, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("HTTP multi-status request failed: %v", resp.Status)
	}

	// TODO: the response can be quite large, support streaming Response elements
	var ms Multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}

	return &ms, nil
}

func (c *Client) Propfind(href string, depth Depth, propfind *Propfind) (*Multistatus, error) {
	req, err := c.NewXMLRequest("PROPFIND", href, propfind)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", depth.String())

	return c.DoMultiStatus(req)
}

// PropfindFlat performs a PROPFIND request with a zero depth.
func (c *Client) PropfindFlat(href string, propfind *Propfind) (*Response, error) {
	ms, err := c.Propfind(href, DepthZero, propfind)
	if err != nil {
		return nil, err
	}

	return ms.Get(href)
}
