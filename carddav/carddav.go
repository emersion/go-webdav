// Package carddav provides a client and server CardDAV implementation.
//
// CardDAV is defined in RFC 6352.
package carddav

import (
	"time"

	"github.com/emersion/go-vcard"
)

type AddressBook struct {
	Path            string
	Name            string
	Description     string
	MaxResourceSize int64
}

type AddressBookQuery struct {
	Props   []string
	AllProp bool

	Limit int // <= 0 means unlimited
}

type AddressBookMultiGet struct {
	Paths []string

	Props   []string
	AllProp bool
}

type AddressObject struct {
	Path    string
	ModTime time.Time
	ETag    string
	Card    vcard.Card
}
