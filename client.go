package webdav

import (
	"encoding/xml"
	"fmt"
	"net/http"

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
	name := xml.Name{"DAV:", "current-user-principal"}
	propfind := internal.NewPropPropfind(name)

	req, err := c.c.NewXMLRequest("PROPFIND", "", propfind)
	if err != nil {
		return "", err
	}

	req.Header.Add("Depth", "0")

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
	if err := propstat.Prop.Decode(&prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}
