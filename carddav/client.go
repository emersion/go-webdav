package carddav

import (
	"encoding/xml"
	"net/http"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

type Client struct {
	*webdav.Client

	ic *internal.Client
}

func NewClient(c *http.Client, endpoint string) (*Client, error) {
	wc, err := webdav.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	ic, err := internal.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{wc, ic}, nil
}

func (c *Client) FindAddressbookHomeSet(principal string) (string, error) {
	name := xml.Name{namespace, "addressbook-home-set"}
	propfind := internal.NewPropPropfind(name)

	resp, err := c.ic.PropfindFlat(principal, propfind)
	if err != nil {
		return "", err
	}

	var prop addressbookHomeSet
	if err := resp.DecodeProp(name, &prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}
