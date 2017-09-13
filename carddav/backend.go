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

type AddressBookInfo struct {
	Name            string
	Description     string
	MaxResourceSize int
}

type AddressObject interface {
	ID() string
	Stat() (os.FileInfo, error) // can return nil, nil
	Card() (vcard.Card, error)
	SetCard(vcard.Card) error
}

type AddressBook interface {
	Info() (*AddressBookInfo, error)
	GetAddressObject(id string) (AddressObject, error)
	ListAddressObjects() ([]AddressObject, error)
	CreateAddressObject(vcard.Card) (AddressObject, error)
}
