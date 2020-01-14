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
	propfind := internal.NewPropPropfind(name)

	req, err := c.c.NewXMLRequest("PROPFIND", "/", propfind)
	if err != nil {
		return "", err
	}

	req.Header.Add("Depth", "0")

	ms, err := c.c.DoMultiStatus(req)
	if err != nil {
		return "", err
	}

	resp, err := ms.Get("/")
	if err != nil {
		return "", err
	}

	var prop currentUserPrincipalProp
	if err := resp.DecodeProp(name, &prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}
