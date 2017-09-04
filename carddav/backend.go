package carddav

// TODO: add context support

import (
	"errors"
	"os"

	"github.com/emersion/go-vcard"
)

var (
	ErrNotFound = errors.New("carddav: not found")
)

type AddressObject interface {
	ID() string
	Card() (vcard.Card, error)
	Stat() (os.FileInfo, error) // can return nil, nil
}

type AddressBook interface {
	GetAddressObject(id string) (AddressObject, error)
	ListAddressObjects() ([]AddressObject, error)
}
