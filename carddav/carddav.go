// Package carddav provides a client and server CardDAV implementation.
//
// CardDAV is defined in RFC 6352.
package carddav

import (
	"github.com/emersion/go-vcard"
)

type AddressBook struct {
	Href            string
	Name            string
	Description     string
	MaxResourceSize int64
}

type AddressBookQuery struct {
	Props []string
}

type AddressBookMultiGet struct {
	Hrefs []string
	Props []string
}

type AddressObject struct {
	Href string
	Card vcard.Card
}
