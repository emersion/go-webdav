package carddav

import (
	"encoding/xml"
)

const namespace = "urn:ietf:params:xml:ns:carddav"

type AddressBook struct {
	Href        string
	Description string
}

var addressBookName = xml.Name{namespace, "addressbook"}

type AddressBookQuery struct {
	AddressDataProps []string
}
