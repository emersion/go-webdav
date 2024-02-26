package caldav

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/emersion/go-webdav/internal"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

var propFindSupportedCalendarComponentRequest = `
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
     <c:supported-calendar-component-set />
  </d:prop>
</d:propfind>
`

var testPropFindSupportedCalendarComponentCases = map[*Calendar][]string{
	&Calendar{Path: "/user/calendars/cal"}:                                                     {"VEVENT"},
	&Calendar{Path: "/user/calendars/cal", SupportedComponentSet: []string{"VTODO"}}:           {"VTODO"},
	&Calendar{Path: "/user/calendars/cal", SupportedComponentSet: []string{"VEVENT", "VTODO"}}: {"VEVENT", "VTODO"},
}

func TestPropFindSupportedCalendarComponent(t *testing.T) {
	for calendar, expected := range testPropFindSupportedCalendarComponentCases {
		req := httptest.NewRequest("PROPFIND", calendar.Path, nil)
		req.Body = io.NopCloser(strings.NewReader(propFindSupportedCalendarComponentRequest))
		req.Header.Set("Content-Type", "application/xml")
		w := httptest.NewRecorder()
		handler := Handler{Backend: &testBackend{calendars: []Calendar{*calendar}}}
		handler.ServeHTTP(w, req)

		res := w.Result()
		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Error(err)
		}
		resp := string(data)
		for _, comp := range expected {
			// Would be nicer to do a proper XML-decoding here, but this is probably good enough for now.
			if !strings.Contains(resp, comp) {
				t.Errorf("Expected component: %v not found in response:\n%v", comp, resp)
			}
		}
	}
}

var propFindCalendarRequest = `
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
	 <d:displayname/>
	 <c:calendar-description/>
     <c:calendar-timezone />
	 <n:calendar-color 
                xmlns:n="http://apple.com/ns/ical/"/>
  </d:prop>
</d:propfind>
`

func TestPropFindCalendar(t *testing.T) {
	calendar := Calendar{
		Path:        "/user/calendars/cal",
		Name:        "Test Calendar",
		Description: "This is a test calendar",
		Timezone:    "BEGIN:VCALENDARfoo",
		Color:       "#DEADBEEF",
	}

	req := httptest.NewRequest("PROPFIND", calendar.Path, nil)
	req.Body = io.NopCloser(strings.NewReader(propFindCalendarRequest))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler := Handler{Backend: &testBackend{calendars: []Calendar{calendar}}}
	handler.ServeHTTP(w, req)

	resp := w.Result()

	var ms internal.MultiStatus
	err := xml.NewDecoder(resp.Body).Decode(&ms)
	assert.NoError(t, err)
	assert.Len(t, ms.Responses, 1)
	assert.Len(t, ms.Responses[0].PropStats, 1)
	assert.Equal(t, 200, ms.Responses[0].PropStats[0].Status.Code)
	assert.Len(t, ms.Responses[0].PropStats[0].Prop.Raw, 4)

	rawDisplayName := ms.Responses[0].PropStats[0].Prop.Get(internal.DisplayNameName)
	rawCalendarDescription := ms.Responses[0].PropStats[0].Prop.Get(calendarDescriptionName)
	rawTimezone := ms.Responses[0].PropStats[0].Prop.Get(calendarTimezoneName)
	rawColor := ms.Responses[0].PropStats[0].Prop.Get(calendarColorName)
	assert.NotNil(t, rawDisplayName)
	assert.NotNil(t, rawCalendarDescription)
	assert.NotNil(t, rawTimezone)
	assert.NotNil(t, rawColor)

	v0 := internal.DisplayName{}
	err = rawDisplayName.Decode(&v0)
	assert.NoError(t, err)
	assert.Equal(t, calendar.Name, v0.Name)

	v1 := calendarDescription{}
	err = rawCalendarDescription.Decode(&v1)
	assert.NoError(t, err)
	assert.Equal(t, calendar.Description, v1.Description)

	v2 := calendarTimezone{}
	err = rawTimezone.Decode(&v2)
	assert.NoError(t, err)
	assert.Equal(t, calendar.Timezone, v2.Timezone)

	v3 := calendarColor{}
	err = rawColor.Decode(&v3)
	assert.NoError(t, err)
	assert.Equal(t, calendar.Color, v3.Color)
}

var propFindUserPrincipal = `
<?xml version="1.0" encoding="UTF-8"?>
<A:propfind xmlns:A="DAV:">
  <A:prop>
    <A:current-user-principal/>
    <A:principal-URL/>
    <A:resourcetype/>
  </A:prop>
</A:propfind>
`

func TestPropFindRoot(t *testing.T) {
	req := httptest.NewRequest("PROPFIND", "/", strings.NewReader(propFindUserPrincipal))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	calendar := &Calendar{}
	handler := Handler{Backend: &testBackend{calendars: []Calendar{*calendar}}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp := string(data)
	if !strings.Contains(resp, `<current-user-principal xmlns="DAV:"><href>/user/</href></current-user-principal>`) {
		t.Errorf("No user-principal returned when doing a PROPFIND against root, response:\n%s", resp)
	}
}

var reportCalendarData = `
<?xml version="1.0" encoding="UTF-8"?>
<B:calendar-multiget xmlns:A="DAV:" xmlns:B="urn:ietf:params:xml:ns:caldav">
  <A:prop>
    <B:calendar-data/>
  </A:prop>
  <A:href>%s</A:href>
</B:calendar-multiget>
`

func TestMultiCalendarBackend(t *testing.T) {
	calendarB := Calendar{Path: "/user/calendars/b", SupportedComponentSet: []string{"VTODO"}}
	calendars := []Calendar{
		Calendar{Path: "/user/calendars/a"},
		calendarB,
	}
	eventSummary := "This is a todo"
	event := ical.NewEvent()
	event.Name = ical.CompToDo
	event.Props.SetText(ical.PropUID, "46bbf47a-1861-41a3-ae06-8d8268c6d41e")
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now())
	event.Props.SetText(ical.PropSummary, eventSummary)
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//xyz Corp//NONSGML PDA Calendar Version 1.0//EN")
	cal.Children = []*ical.Component{
		event.Component,
	}
	object := CalendarObject{
		Path: "/user/calendars/b/test.ics",
		Data: cal,
	}
	req := httptest.NewRequest("PROPFIND", "/user/calendars/", strings.NewReader(propFindUserPrincipal))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler := Handler{Backend: &testBackend{
		calendars: calendars,
		objectMap: map[string][]CalendarObject{
			calendarB.Path: []CalendarObject{object},
		},
	}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp := string(data)
	for _, calendar := range calendars {
		if !strings.Contains(resp, fmt.Sprintf(`<response xmlns="DAV:"><href>%s</href>`, calendar.Path)) {
			t.Errorf("Calendar: %v not returned in PROPFIND, response:\n%s", calendar, resp)
		}
	}

	// Now do a PROPFIND for the last calendar
	req = httptest.NewRequest("PROPFIND", calendarB.Path, strings.NewReader(propFindSupportedCalendarComponentRequest))
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp = string(data)
	if !strings.Contains(resp, "VTODO") {
		t.Errorf("Expected component: VTODO not found in response:\n%v", resp)
	}
	if !strings.Contains(resp, object.Path) {
		t.Errorf("Expected calendar object: %v not found in response:\n%v", object, resp)
	}

	// Now do a REPORT to get the actual data for the event
	req = httptest.NewRequest("REPORT", calendarB.Path, strings.NewReader(fmt.Sprintf(reportCalendarData, object.Path)))
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp = string(data)
	if !strings.Contains(resp, fmt.Sprintf("SUMMARY:%s", eventSummary)) {
		t.Errorf("ICAL content not properly returned in response:\n%v", resp)
	}
}

var mkcolRequestData = `
<?xml version='1.0' encoding='UTF-8' ?>
<mkcol
    xmlns="DAV:"
    xmlns:CAL="urn:ietf:params:xml:ns:caldav"
    xmlns:CARD="urn:ietf:params:xml:ns:carddav">
    <set>
        <prop>
            <resourcetype>
                <collection />
                <CAL:calendar />
            </resourcetype>
            <displayname>Test calendar</displayname>
            <CAL:calendar-description>A calendar for testing</CAL:calendar-description>
            <n0:calendar-color
                xmlns:n0="http://apple.com/ns/ical/">#009688FF
            </n0:calendar-color>
            <CAL:calendar-timezone>
                <![CDATA[BEGIN:VCALENDAR
BEGIN:VTIMEZONE
TZID:Europe/Berlin
LAST-MODIFIED:20230104T023643Z
TZURL:https://www.tzurl.org/zoneinfo/Europe/Berlin
X-LIC-LOCATION:Europe/Berlin
X-PROLEPTIC-TZNAME:LMT
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+005328
TZOFFSETTO:+0100
DTSTART:18930401T000632
END:STANDARD
BEGIN:DAYLIGHT
TZNAME:CEST
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
DTSTART:19160430T230000
RDATE:19400401T020000
RDATE:19430329T020000
RDATE:19460414T020000
RDATE:19470406T030000
RDATE:19480418T020000
RDATE:19490410T020000
RDATE:19800406T020000
END:DAYLIGHT
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
DTSTART:19161001T010000
RDATE:19421102T030000
RDATE:19431004T030000
RDATE:19441002T030000
RDATE:19451118T030000
RDATE:19461007T030000
END:STANDARD
BEGIN:DAYLIGHT
TZNAME:CEST
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
DTSTART:19170416T020000
RRULE:FREQ=YEARLY;UNTIL=19180415T010000Z;BYMONTH=4;BYDAY=3MO
END:DAYLIGHT
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
DTSTART:19170917T030000
RRULE:FREQ=YEARLY;UNTIL=19180916T010000Z;BYMONTH=9;BYDAY=3MO
END:STANDARD
BEGIN:DAYLIGHT
TZNAME:CEST
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
DTSTART:19440403T020000
RRULE:FREQ=YEARLY;UNTIL=19450402T010000Z;BYMONTH=4;BYDAY=1MO
END:DAYLIGHT
BEGIN:DAYLIGHT
TZNAME:CEMT
TZOFFSETFROM:+0200
TZOFFSETTO:+0300
DTSTART:19450524T010000
RDATE:19470511T020000
END:DAYLIGHT
BEGIN:DAYLIGHT
TZNAME:CEST
TZOFFSETFROM:+0300
TZOFFSETTO:+0200
DTSTART:19450924T030000
RDATE:19470629T030000
END:DAYLIGHT
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0100
TZOFFSETTO:+0100
DTSTART:19460101T000000
RDATE:19800101T000000
END:STANDARD
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
DTSTART:19471005T030000
RRULE:FREQ=YEARLY;UNTIL=19491002T010000Z;BYMONTH=10;BYDAY=1SU
END:STANDARD
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
DTSTART:19800928T030000
RRULE:FREQ=YEARLY;UNTIL=19950924T010000Z;BYMONTH=9;BYDAY=-1SU
END:STANDARD
BEGIN:DAYLIGHT
TZNAME:CEST
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
DTSTART:19810329T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU
END:DAYLIGHT
BEGIN:STANDARD
TZNAME:CET
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
DTSTART:19961027T030000
RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU
END:STANDARD
END:VTIMEZONE
END:VCALENDAR
]]>
            </CAL:calendar-timezone>
            <CAL:supported-calendar-component-set>
                <CAL:comp name="VEVENT" />
                <CAL:comp name="VTODO" />
                <CAL:comp name="VJOURNAL" />
            </CAL:supported-calendar-component-set>
        </prop>
    </set>
</mkcol>`

func TestCreateCalendar(t *testing.T) {
	tb := testBackend{
		calendars: nil,
		objectMap: nil,
	}
	b := backend{
		Backend: &tb,
		Prefix:  "/dav",
	}
	req := httptest.NewRequest("MKCOL", "/dav/calendars/user0/test-calendar", strings.NewReader(mkcolRequestData))
	req.Header.Set("Content-Type", "application/xml")

	err := b.Mkcol(req)
	assert.NoError(t, err)
	assert.Len(t, tb.calendars, 1)
	c := tb.calendars[0]
	assert.Equal(t, "Test calendar", c.Name)
	assert.Equal(t, "/dav/calendars/user0/test-calendar", c.Path)
	assert.Equal(t, "A calendar for testing", c.Description)
	assert.Equal(t, "#009688FF", c.Color)
	assert.Equal(t, []string{"VEVENT", "VTODO", "VJOURNAL"}, c.SupportedComponentSet)
	assert.Contains(t, c.Timezone, "BEGIN:VCALENDAR")
}

var mkcolRequestDataMinimalBody = `
<?xml version='1.0' encoding='UTF-8' ?>
<mkcol
    xmlns="DAV:"
    xmlns:CAL="urn:ietf:params:xml:ns:caldav"
    xmlns:CARD="urn:ietf:params:xml:ns:carddav">
    <set>
        <prop>
            <resourcetype>
                <collection />
                <CAL:calendar />
            </resourcetype>
            <displayname>Test calendar</displayname>
        </prop>
    </set>
</mkcol>`

func TestCreateCalendarMinimalBody(t *testing.T) {
	tb := testBackend{
		calendars: nil,
		objectMap: nil,
	}
	b := backend{
		Backend: &tb,
		Prefix:  "/dav",
	}
	req := httptest.NewRequest("MKCOL", "/dav/calendars/user0/test-calendar", strings.NewReader(mkcolRequestDataMinimalBody))
	req.Header.Set("Content-Type", "application/xml")

	err := b.Mkcol(req)
	assert.NoError(t, err)
	assert.Len(t, tb.calendars, 1)
	c := tb.calendars[0]
	assert.Equal(t, "Test calendar", c.Name)
	assert.Equal(t, "/dav/calendars/user0/test-calendar", c.Path)
	assert.Equal(t, "", c.Description)
	assert.Equal(t, "", c.Color)
	assert.Equal(t, []string{}, c.SupportedComponentSet)
	assert.Equal(t, "", c.Timezone)
}

type testBackend struct {
	calendars []Calendar
	objectMap map[string][]CalendarObject
}

func (t *testBackend) CreateCalendar(ctx context.Context, calendar *Calendar) error {
	t.calendars = append(t.calendars, *calendar)
	return nil
}

func (t *testBackend) ListCalendars(ctx context.Context) ([]Calendar, error) {
	return t.calendars, nil
}

func (t *testBackend) GetCalendar(ctx context.Context, path string) (*Calendar, error) {
	for _, cal := range t.calendars {
		if cal.Path == path {
			return &cal, nil
		}
	}
	return nil, fmt.Errorf("calendar for path: %s not found", path)
}

func (t *testBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/user/calendars/", nil
}

func (t *testBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/user/", nil
}

func (t *testBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	return nil
}

func (t *testBackend) GetCalendarObject(ctx context.Context, path string, req *CalendarCompRequest) (*CalendarObject, error) {
	for _, objs := range t.objectMap {
		for _, obj := range objs {
			if obj.Path == path {
				return &obj, nil
			}
		}
	}
	return nil, fmt.Errorf("couldn't find calendar object at: %s", path)
}

func (t *testBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *PutCalendarObjectOptions) (string, error) {
	return "", nil
}

func (t *testBackend) ListCalendarObjects(ctx context.Context, path string, req *CalendarCompRequest) ([]CalendarObject, error) {
	return t.objectMap[path], nil
}

func (t *testBackend) QueryCalendarObjects(ctx context.Context, path string, query *CalendarQuery) ([]CalendarObject, error) {
	return nil, nil
}
