package webdav

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/emersion/go-webdav/internal"
)

type Client struct {
	c *internal.Client
}

func NewClient(c *http.Client, endpoint string) (*Client, error) {
	ic, err := internal.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{ic}, nil
}

func (c *Client) FindCurrentUserPrincipal() (string, error) {
	r := strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:current-user-principal/>
  </D:prop>
</D:propfind>
`)

	req, err := c.c.NewRequest("PROPFIND", "", r)
	if err != nil {
		return "", err
	}

	req.Header.Add("Depth", "0")
	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")

	resps, err := c.c.DoMultiStatus(req)
	if err != nil {
		return "", err
	}

	if len(resps) != 1 {
		return "", fmt.Errorf("expected exactly one response in multistatus, got %v", len(resps))
	}
	resp := &resps[0]

	// TODO: handle propstats with errors
	if len(resp.Propstats) != 1 {
		return "", fmt.Errorf("expected exactly one propstat in response")
	}
	propstat := &resp.Propstats[0]

	var prop currentUserPrincipalProp
	if err := xml.NewTokenDecoder(propstat.Prop.TokenReader()).Decode(&prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}
