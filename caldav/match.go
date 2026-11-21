package caldav

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Filter returns the filtered list of calendar objects matching the provided query.
// A nil query will return the full list of calendar objects.
func Filter(query *CalendarQuery, cos []CalendarObject) ([]CalendarObject, error) {
	if query == nil {
		// FIXME: should we always return a copy of the provided slice?
		return cos, nil
	}

	var out []CalendarObject
	for _, co := range cos {
		ok, err := Match(query.CompFilter, &co)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		// TODO properties are not currently filtered even if requested
		out = append(out, co)
	}
	return out, nil
}

// Match reports whether the provided CalendarObject matches the query.
func Match(query CompFilter, co *CalendarObject) (matched bool, err error) {
	if co.Data == nil || co.Data.Component == nil {
		panic("request to process empty calendar object")
	}
	return match(query, co.Data.Component)
}

func match(filter CompFilter, comp *ical.Component) (bool, error) {
	if comp.Name != filter.Name {
		return filter.IsNotDefined, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchCompTimeRange(filter.Start, filter.End, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	for _, compFilter := range filter.Comps {
		match, err := matchCompFilter(compFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	for _, propFilter := range filter.Props {
		match, err := matchPropFilter(propFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func matchCompFilter(filter CompFilter, comp *ical.Component) (bool, error) {
	var matches []*ical.Component

	for _, child := range comp.Children {
		match, err := match(filter, child)
		if err != nil {
			return false, err
		} else if match {
			matches = append(matches, child)
		}
	}
	if len(matches) == 0 {
		return filter.IsNotDefined, nil
	}
	return true, nil
}

func matchPropFilter(filter PropFilter, comp *ical.Component) (bool, error) {
	// TODO: this only matches first field, there can be multiple
	field := comp.Props.Get(filter.Name)
	if field == nil {
		return filter.IsNotDefined, nil
	}

	for _, paramFilter := range filter.ParamFilter {
		if !matchParamFilter(paramFilter, field) {
			return false, nil
		}
	}

	var zeroDate time.Time
	if filter.Start != zeroDate {
		match, err := matchPropTimeRange(filter.Start, filter.End, field)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	} else if filter.TextMatch != nil {
		if !matchTextMatch(*filter.TextMatch, field.Value) {
			return false, nil
		}
		return true, nil
	}
	// empty prop-filter, property exists
	return true, nil
}

func matchCompTimeRange(start, end time.Time, comp *ical.Component) (bool, error) {
	// See https://datatracker.ietf.org/doc/html/rfc4791#section-9.9
	// The "start" attribute specifies the inclusive start of the time range,
	// and the "end" attribute specifies the non-inclusive end of the time range.
	// Both attributes MUST be specified as "date with UTC time" value.

	// evaluate recurring components
	rset, err := comp.RecurrenceSet(time.UTC)
	if err != nil {
		return false, err
	}
	if rset != nil {
		// return len(rset.Between(start, end, true)) > 0, nil
		// if start is zero then rset.After(zero) should work

		// TODO: first_after_start only looks at DTSTART yielding wrong behaviour;
		// an event can start before interval [start,end) but still intersect the interval;
		// in this case it should be matched by according to RFC 4791.
		//
		// "The CALDAV:time-range XML element specifies that for a
		// given calendaring REPORT request, the server MUST only return the
		// calendar object resources that, depending on the context, have a
		// component or property whose value intersects a specified time
		// range."
		//
		// OPTIMIZATION: would make slightly more efficient code,
		// i.e., fewer passes over rset iterator,
		// if rset.Iterator's next() function was exported as Next()
		// and the following code block was rewritten
		if first_after_start := rset.After(start, true); first_after_start.IsZero() {
			return false, nil
		} else if end.IsZero() || first_after_start.Before(end) {
			return true, nil
		} else {
			return false, nil
		}
	}

	// TODO handle more than just events
	if comp.Name != ical.CompEvent {
		return false, nil
	}
	event := ical.Event{Component: comp}

	eventStart, err := event.DateTimeStart(time.UTC)
	if err != nil {
		return false, err
	}
	eventEnd, err := event.DateTimeEnd(time.UTC)
	if err != nil {
		return false, err
	}
	duration_zero := eventStart.Equal(eventEnd)

	// test if [eventStart, eventEnd) intersects [start, end)
	// special handling if duration_zero;
	// in that case check if eventStart is contained in [start,end)
	//
	// S_E compare event start versus filter end
	// E_S compare event end versus filter start
	//
	// refer to table https://datatracker.ietf.org/doc/html/rfc4791#section-9.9
	//
	if S_E := eventStart.Compare(end); start.IsZero() && S_E < 0 {
		return true, nil
	} else if E_S := eventEnd.Compare(start); end.IsZero() && (E_S > 0 || (duration_zero && E_S >= 0)) {
		return true, nil
	} else if (S_E < 0 && E_S > 0) || (duration_zero && E_S >= 0 && S_E < 0) {
		return true, nil
	} else {
		return false, nil
	}
}

func matchPropTimeRange(start, end time.Time, field *ical.Prop) (bool, error) {
	// See https://datatracker.ietf.org/doc/html/rfc4791#section-9.9

	ptime, err := field.DateTime(start.Location())
	if err != nil {
		return false, err
	}
	if ptime.After(start) && (end.IsZero() || ptime.Before(end)) {
		return true, nil
	}
	return false, nil
}

func matchParamFilter(filter ParamFilter, field *ical.Prop) bool {
	// TODO there can be multiple values
	value := field.Params.Get(filter.Name)
	if value == "" {
		return filter.IsNotDefined
	} else if filter.IsNotDefined {
		return false
	}
	if filter.TextMatch != nil {
		return matchTextMatch(*filter.TextMatch, value)
	}
	return true
}

func matchTextMatch(txt TextMatch, value string) bool {
	// TODO: handle text-match collation attribute
	match := strings.Contains(value, txt.Text)
	if txt.NegateCondition {
		match = !match
	}
	return match
}
