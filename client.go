package webdav

import (
	"encoding/xml"
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
	propfind := internal.NewPropNamePropfind(name)

	resp, err := c.c.PropfindFlat("/", propfind)
	if err != nil {
		return "", err
	}

	var prop currentUserPrincipal
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}
