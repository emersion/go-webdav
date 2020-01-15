package carddav

import (
	"bytes"
	"encoding/xml"
	"net/http"

	"github.com/emersion/go-vcard"
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

func (c *Client) FindAddressBookHomeSet(principal string) (string, error) {
	name := xml.Name{namespace, "addressbook-home-set"}
	propfind := internal.NewPropNamePropfind(name)

	resp, err := c.ic.PropfindFlat(principal, propfind)
	if err != nil {
		return "", err
	}

	var prop addressbookHomeSet
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}

	return prop.Href, nil
}

func (c *Client) FindAddressBooks(addressBookHomeSet string) ([]AddressBook, error) {
	resTypeName := xml.Name{"DAV:", "resourcetype"}
	descName := xml.Name{namespace, "addressbook-description"}
	propfind := internal.NewPropNamePropfind(resTypeName, descName)

	req, err := c.ic.NewXMLRequest("PROPFIND", addressBookHomeSet, propfind)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", "1")

	ms, err := c.ic.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	l := make([]AddressBook, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		href, err := resp.Href()
		if err != nil {
			return nil, err
		}

		var resTypeProp internal.ResourceType
		if err := resp.DecodeProp(&resTypeProp); err != nil {
			return nil, err
		}
		if !resTypeProp.Is(addressBookName) {
			continue
		}

		var descProp addressbookDescription
		if err := resp.DecodeProp(&descProp); err != nil {
			return nil, err
		}

		l = append(l, AddressBook{
			Href:        href,
			Description: descProp.Data,
		})
	}

	return l, nil
}

func (c *Client) QueryAddressBook(addressBook string, query *AddressBookQuery) ([]Address, error) {
	var addrDataReq addressDataReq
	for _, name := range query.Props {
		addrDataReq.Props = append(addrDataReq.Props, prop{Name: name})
	}

	propReq, err := internal.EncodeProp(&addrDataReq)
	if err != nil {
		return nil, err
	}

	addressbookQuery := addressbookQuery{Prop: propReq}

	req, err := c.ic.NewXMLRequest("REPORT", addressBook, &addressbookQuery)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", "1")

	ms, err := c.ic.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	addrs := make([]Address, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		href, err := resp.Href()
		if err != nil {
			return nil, err
		}

		var addrData addressDataResp
		if err := resp.DecodeProp(&addrData); err != nil {
			return nil, err
		}

		r := bytes.NewReader(addrData.Data)
		card, err := vcard.NewDecoder(r).Decode()
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, Address{
			Href: href,
			Card: card,
		})
	}

	return addrs, nil
}
