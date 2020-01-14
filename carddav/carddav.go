package carddav

import (
	"encoding/xml"

	"github.com/emersion/go-vcard"
)

const namespace = "urn:ietf:params:xml:ns:carddav"

type AddressBook struct {
	Href        string
	Description string
}

var addressBookName = xml.Name{namespace, "addressbook"}

type AddressBookQuery struct {
	Props []string
}

type Address struct {
	Href string
	Card vcard.Card
}
