package webdav

import (
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

func (c *Client) SetBasicAuth(username, password string) {
	c.c.SetBasicAuth(username, password)
}

func (c *Client) FindCurrentUserPrincipal() (string, error) {
	propfind := internal.NewPropNamePropfind(internal.CurrentUserPrincipalName)

	resp, err := c.c.PropfindFlat("/", propfind)
	if err != nil {
		return "", err
	}

	var prop internal.CurrentUserPrincipal
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}
	if prop.Unauthenticated != nil {
		return "", fmt.Errorf("webdav: unauthenticated")
	}

	return prop.Href, nil
}
