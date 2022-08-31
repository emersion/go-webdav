package caldav

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

var dateFormat = "20060102T150405Z"

func toDate(t *testing.T, date string) time.Time {
	res, err := time.ParseInLocation(dateFormat, date, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Test data taken from https://datatracker.ietf.org/doc/html/rfc4791#appendix-B
// TODO add missing data
func TestFilter(t *testing.T) {
	newCO := func(str string) CalendarObject {
		cal, err := ical.NewDecoder(strings.NewReader(str)).Decode()
		if err != nil {
			t.Fatal(err)
		}
		return CalendarObject{
			Data: cal,
		}
	}

	event1 := newCO(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
DTSTAMP:20060206T001102Z
DTSTART;TZID=US/Eastern:20060102T100000
DURATION:PT1H
SUMMARY:Event #1
Description:Go Steelers!
UID:74855313FA803DA593CD579A@example.com
END:VEVENT
END:VCALENDAR`)

	event2 := newCO(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
DTSTAMP:20060206T001121Z
DTSTART;TZID=US/Eastern:20060102T120000
DURATION:PT1H
RRULE:FREQ=DAILY;COUNT=5
SUMMARY:Event #2
UID:00959BC664CA650E933C892C@example.com
END:VEVENT
BEGIN:VEVENT
DTSTAMP:20060206T001121Z
DTSTART;TZID=US/Eastern:20060104T140000
DURATION:PT1H
RECURRENCE-ID;TZID=US/Eastern:20060104T120000
SUMMARY:Event #2 bis
UID:00959BC664CA650E933C892C@example.com
END:VEVENT
BEGIN:VEVENT
DTSTAMP:20060206T001121Z
DTSTART;TZID=US/Eastern:20060106T140000
DURATION:PT1H
RECURRENCE-ID;TZID=US/Eastern:20060106T120000
SUMMARY:Event #2 bis bis
UID:00959BC664CA650E933C892C@example.com
END:VEVENT
END:VCALENDAR`)

	event3 := newCO(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
ATTENDEE;PARTSTAT=ACCEPTED;ROLE=CHAIR:mailto:cyrus@example.com
ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:lisa@example.com
DTSTAMP:20060206T001220Z
DTSTART;TZID=US/Eastern:20060104T100000
DURATION:PT1H
LAST-MODIFIED:20060206T001330Z
ORGANIZER:mailto:cyrus@example.com
SEQUENCE:1
STATUS:TENTATIVE
SUMMARY:Event #3
UID:DC6C50A017428C5216A2F1CD@example.com
END:VEVENT
END:VCALENDAR`)

	todo1 := newCO(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTODO
DTSTAMP:20060205T235335Z
DUE;VALUE=DATE:20060104
STATUS:NEEDS-ACTION
SUMMARY:Task #1
UID:DDDEEB7915FA61233B861457@example.com
BEGIN:VALARM
ACTION:AUDIO
TRIGGER;RELATED=START:-PT10M
END:VALARM
END:VTODO
END:VCALENDAR`)

	for _, tc := range []struct {
		name  string
		query *CalendarQuery
		addrs []CalendarObject
		want  []CalendarObject
		err   error
	}{
		{
			name:  "nil-query",
			query: nil,
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event1, event2, event3, todo1},
		},
		{
			// https://datatracker.ietf.org/doc/html/rfc4791#section-7.8.8
			name: "events only",
			query: &CalendarQuery{
				CompFilter: CompFilter{
					Name: "VCALENDAR",
					Comps: []CompFilter{
						CompFilter{
							Name: "VEVENT",
						},
					},
				},
			},
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event1, event2, event3},
		},
		{
			// https://datatracker.ietf.org/doc/html/rfc4791#section-7.8.1
			name: "events in time range",
			query: &CalendarQuery{
				CompFilter: CompFilter{
					Name: "VCALENDAR",
					Comps: []CompFilter{
						CompFilter{
							Name:  "VEVENT",
							Start: toDate(t, "20060104T000000Z"),
							End:   toDate(t, "20060105T000000Z"),
						},
					},
				},
			},
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event2, event3},
		},
		{
			// https://datatracker.ietf.org/doc/html/rfc4791#section-7.8.6
			name: "events by UID",
			query: &CalendarQuery{
				CompFilter: CompFilter{
					Name: "VCALENDAR",
					Comps: []CompFilter{
						CompFilter{
							Name: "VEVENT",
							Props: []PropFilter{{
								Name: "UID",
								TextMatch: &TextMatch{
									Text: "DC6C50A017428C5216A2F1CD@example.com",
								},
							}},
						},
					},
				},
			},
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event3},
		},
		{
			// https://datatracker.ietf.org/doc/html/rfc4791#section-7.8.6
			name: "events by description substring",
			query: &CalendarQuery{
				CompFilter: CompFilter{
					Name: "VCALENDAR",
					Comps: []CompFilter{
						CompFilter{
							Name: "VEVENT",
							Props: []PropFilter{{
								Name: "Description",
								TextMatch: &TextMatch{
									Text: "Steelers",
								},
							}},
						},
					},
				},
			},
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event1},
		},
		{
			// Query a time range that only returns a result if recurrence is properly evaluated.
			name: "recurring events in time range",
			query: &CalendarQuery{
				CompFilter: CompFilter{
					Name: "VCALENDAR",
					Comps: []CompFilter{
						CompFilter{
							Name:  "VEVENT",
							Start: toDate(t, "20060103T000000Z"),
							End:   toDate(t, "20060104T000000Z"),
						},
					},
				},
			},
			addrs: []CalendarObject{event1, event2, event3, todo1},
			want:  []CalendarObject{event2},
		},
		// TODO add more examples
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Filter(tc.query, tc.addrs)
			switch {
			case err != nil && tc.err == nil:
				t.Fatalf("unexpected error: %+v", err)
			case err != nil && tc.err != nil:
				if got, want := err.Error(), tc.err.Error(); got != want {
					t.Fatalf("invalid error:\ngot= %q\nwant=%q", got, want)
				}
			case err == nil && tc.err != nil:
				t.Fatalf("expected an error:\ngot= %+v\nwant=%+v", err, tc.err)
			case err == nil && tc.err == nil:
				if got, want := got, tc.want; !reflect.DeepEqual(got, want) {
					t.Fatalf("invalid filter values:\ngot= %+v\nwant=%+v", got, want)
				}
			}
		})
	}
}
