// Package carddav provides a client and server CardDAV implementation.
//
// CardDAV is defined in RFC 6352.
package carddav

import (
	"github.com/emersion/go-vcard"
)

type AddressBook struct {
	Path            string
	Name            string
	Description     string
	MaxResourceSize int64
}

type AddressBookQuery struct {
	Props []string
}

type AddressBookMultiGet struct {
	Paths []string
	Props []string
}

type AddressObject struct {
	Path string
	Card vcard.Card
}
