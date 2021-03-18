package carddav

import (
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"
)

// Filter returns the filtered list of address objects matching the provided query.
// A nil query will return the full list of address objects.
func Filter(query *AddressBookQuery, aos []AddressObject) ([]AddressObject, error) {
	if query == nil {
		// FIXME: should we always return a copy of the provided slice?
		return aos, nil
	}

	n := query.Limit
	if n <= 0 {
		n = len(aos)
	}
	out := make([]AddressObject, 0, n)
	for _, ao := range aos {
		ok, err := Match(query, ao)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, ao)
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// Match reports whether the provided AddressObject matches the query.
func Match(query *AddressBookQuery, ao AddressObject) (matched bool, err error) {
	if query == nil {
		return true, nil
	}

	if query.DataRequest.AllProp {
		for _, name := range query.DataRequest.Props {
			field := ao.Card.Get(name)
			if field == nil {
				// missing required property.
				return false, fmt.Errorf("missing property %q", name)
			}
		}
	}

	switch query.FilterTest {
	default:
		return false, fmt.Errorf("unknown query filter test %q", query.FilterTest)

	case FilterAnyOf, "":
		for _, prop := range query.PropFilters {
			ok, err := matchPropFilter(prop, ao)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil

	case FilterAllOf:
		for _, prop := range query.PropFilters {
			ok, err := matchPropFilter(prop, ao)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}
}

func matchPropFilter(prop PropFilter, ao AddressObject) (bool, error) {
	field := ao.Card.Get(prop.Name)
	if field == nil {
		// assume AddressBookQuery.DataRequest.AllProp is false
		return false, nil
	}

	// TODO: handle carddav.PropFilter.IsNotDefined.
	// TODO: handle carddav.PropFilter.Params.

	switch prop.Test {
	default:
		return false, fmt.Errorf("unknown property filter test %q", prop.Test)

	case FilterAnyOf, "":
		for _, txt := range prop.TextMatches {
			ok, err := matchTextMatch(txt, field)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil

	case FilterAllOf:
		for _, txt := range prop.TextMatches {
			ok, err := matchTextMatch(txt, field)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}
}

func matchTextMatch(txt TextMatch, field *vcard.Field) (bool, error) {
	// TODO: handle carddav.TextMatch.IsNotDefined.
	var ok bool
	switch txt.MatchType {
	default:
		return false, fmt.Errorf("unknown textmatch type %q", txt.MatchType)

	case MatchEquals:
		ok = txt.Text == field.Value

	case MatchContains, "":
		ok = strings.Contains(field.Value, txt.Text)

	case MatchStartsWith:
		ok = strings.HasPrefix(field.Value, txt.Text)

	case MatchEndsWith:
		ok = strings.HasSuffix(field.Value, txt.Text)
	}

	if txt.NegateCondition {
		ok = !ok
	}
	return ok, nil
}
