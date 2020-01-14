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

type Client struct {
	http     *http.Client
	endpoint *url.URL
}

func NewClient(c *http.Client, endpoint string) (*Client, error) {
	if c == nil {
		c = http.DefaultClient
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{c, u}, nil
}

func (c *Client) NewRequest(method string, href string, body io.Reader) (*http.Request, error) {
	u := url.URL{
		Scheme: c.endpoint.Scheme,
		User:   c.endpoint.User,
		Host:   c.endpoint.Host,
		Path:   path.Join(c.endpoint.Path, href),
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
	// TODO: remove this quirk
	req.SetBasicAuth("emersion", "")
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

// PropfindFlat performs a PROPFIND request with a zero depth.
func (c *Client) PropfindFlat(href string, propfind *Propfind) (*Response, error) {
	req, err := c.NewXMLRequest("PROPFIND", href, propfind)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", "0")

	ms, err := c.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	return ms.Get(href)
}
