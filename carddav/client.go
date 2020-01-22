package carddav

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

// Discover performs a DNS-based CardDAV service discovery as described in
// RFC 6352 section 11. It returns the URL to the CardDAV server.
func Discover(domain string) (string, error) {
	// Only lookup carddavs (not carddav), plaintext connections are insecure
	_, addrs, err := net.LookupSRV("carddavs", "tcp", domain)
	if dnsErr, ok := err.(*net.DNSError); ok {
		if dnsErr.IsTemporary {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	if len(addrs) == 0 {
		return "", fmt.Errorf("carddav: domain doesn't have an SRV record")
	}
	addr := addrs[0]

	target := strings.TrimSuffix(addr.Target, ".")
	if target == "" {
		return "", nil
	}

	u := url.URL{Scheme: "https"}
	if addr.Port == 443 {
		u.Host = addr.Target
	} else {
		u.Host = fmt.Sprintf("%v:%v", target, addr.Port)
	}
	return u.String(), nil
}

// Client provides access to a remote CardDAV server.
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

func (c *Client) SetBasicAuth(username, password string) {
	c.Client.SetBasicAuth(username, password)
	c.ic.SetBasicAuth(username, password)
}

func (c *Client) FindAddressBookHomeSet(principal string) (string, error) {
	propfind := internal.NewPropNamePropfind(addressBookHomeSetName)
	resp, err := c.ic.PropfindFlat(principal, propfind)
	if err != nil {
		return "", err
	}

	var prop addressbookHomeSet
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}

	return prop.Href.Path, nil
}

func (c *Client) FindAddressBooks(addressBookHomeSet string) ([]AddressBook, error) {
	propfind := internal.NewPropNamePropfind(
		internal.ResourceTypeName,
		internal.DisplayNameName,
		addressBookDescriptionName,
		maxResourceSizeName,
	)
	ms, err := c.ic.Propfind(addressBookHomeSet, internal.DepthOne, propfind)
	if err != nil {
		return nil, err
	}

	l := make([]AddressBook, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
		if err != nil {
			return nil, err
		}

		var resType internal.ResourceType
		if err := resp.DecodeProp(&resType); err != nil {
			return nil, err
		}
		if !resType.Is(addressBookName) {
			continue
		}

		var desc addressbookDescription
		if err := resp.DecodeProp(&desc); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var dispName internal.DisplayName
		if err := resp.DecodeProp(&dispName); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var maxResSize maxResourceSize
		if err := resp.DecodeProp(&maxResSize); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}
		if maxResSize.Size < 0 {
			return nil, fmt.Errorf("carddav: max-resource-size must be a positive integer")
		}

		l = append(l, AddressBook{
			Path:            path,
			Name:            dispName.Name,
			Description:     desc.Description,
			MaxResourceSize: maxResSize.Size,
		})
	}

	return l, nil
}

func decodeAddressList(ms *internal.Multistatus) ([]AddressObject, error) {
	addrs := make([]AddressObject, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
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

		addrs = append(addrs, AddressObject{
			Path: path,
			Card: card,
		})
	}

	return addrs, nil
}

func (c *Client) QueryAddressBook(addressBook string, query *AddressBookQuery) ([]AddressObject, error) {
	var addrDataReq addressDataReq
	if query != nil {
		for _, name := range query.Props {
			addrDataReq.Props = append(addrDataReq.Props, prop{Name: name})
		}
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

	return decodeAddressList(ms)
}

func (c *Client) MultiGetAddressBook(path string, multiGet *AddressBookMultiGet) ([]AddressObject, error) {
	var addrDataReq addressDataReq
	if multiGet != nil {
		for _, name := range multiGet.Props {
			addrDataReq.Props = append(addrDataReq.Props, prop{Name: name})
		}
	}

	propReq, err := internal.EncodeProp(&addrDataReq)
	if err != nil {
		return nil, err
	}

	addressbookMultiget := addressbookMultiget{Prop: propReq}

	if multiGet == nil || len(multiGet.Paths) == 0 {
		href := internal.Href{Path: path}
		addressbookMultiget.Hrefs = []internal.Href{href}
	} else {
		addressbookMultiget.Hrefs = make([]internal.Href, len(multiGet.Paths))
		for i, p := range multiGet.Paths {
			addressbookMultiget.Hrefs[i] = internal.Href{Path: p}
		}
	}

	req, err := c.ic.NewXMLRequest("REPORT", path, &addressbookMultiget)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", "1")

	ms, err := c.ic.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	return decodeAddressList(ms)
}
