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
	DataRequest AddressDataRequest

	PropFilters []PropFilter
	FilterTest  FilterTest // defaults to FilterAnyOf

	Limit int // <= 0 means unlimited
}

type AddressDataRequest struct {
	Props   []string
	AllProp bool
}

type PropFilter struct {
	Name string
	Test FilterTest // defaults to FilterAnyOf

	// if IsNotDefined is set, TextMatches and Params need to be unset
	IsNotDefined bool
	TextMatches  []TextMatch
	Params       []ParamFilter
}

type ParamFilter struct {
	Name string

	// if IsNotDefined is set, TextMatch needs to be unset
	IsNotDefined bool
	TextMatch    *TextMatch
}

type TextMatch struct {
	Text            string
	NegateCondition bool
	MatchType       MatchType // defaults to MatchContains
}

type FilterTest string

const (
	FilterAnyOf FilterTest = "anyof"
	FilterAllOf FilterTest = "allof"
)

type MatchType string

const (
	MatchEquals     MatchType = "equals"
	MatchContains   MatchType = "contains"
	MatchStartsWith MatchType = "starts-with"
	MatchEndsWith   MatchType = "ends-with"
)

type AddressBookMultiGet struct {
	Paths       []string
	DataRequest AddressDataRequest
}

type AddressObject struct {
	Path    string
	ModTime time.Time
	ETag    string
	Card    vcard.Card
}
