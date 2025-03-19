package caldav

import (
	"context"
	"fmt"
	"io"
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
	&Calendar{Path: "/user/calendars/cal"}:                                                     []string{"VEVENT"},
	&Calendar{Path: "/user/calendars/cal", SupportedComponentSet: []string{"VTODO"}}:           []string{"VTODO"},
	&Calendar{Path: "/user/calendars/cal", SupportedComponentSet: []string{"VEVENT", "VTODO"}}: []string{"VEVENT", "VTODO"},
}

func TestPropFindSupportedCalendarComponent(t *testing.T) {
	for calendar, expected := range testPropFindSupportedCalendarComponentCases {
		req := httptest.NewRequest("PROPFIND", calendar.Path, nil)
		req.Body = io.NopCloser(strings.NewReader(propFindSupportedCalendarComponentRequest))
		req.Header.Set("Content-Type", "application/xml")
		w := httptest.NewRecorder()
		handler := Handler{Backend: testBackend{calendars: []Calendar{*calendar}}}
		handler.ServeHTTP(w, req)

		res := w.Result()
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
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
	handler := Handler{Backend: testBackend{calendars: []Calendar{*calendar}}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
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
	handler := Handler{Backend: testBackend{
		calendars: calendars,
		objectMap: map[string][]CalendarObject{
			calendarB.Path: []CalendarObject{object},
		},
	}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
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
	data, err = io.ReadAll(res.Body)
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
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp = string(data)
	if !strings.Contains(resp, fmt.Sprintf("SUMMARY:%s", eventSummary)) {
		t.Errorf("ICAL content not properly returned in response:\n%v", resp)
	}
}

var propFindAllProp = `
<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:allprop/>
</D:propfind>
`

var reportTest1 = `
<?xml version="1.0" encoding="utf-8"?>
<C:calendar-query xmlns:C="urn:ietf:params:xml:ns:caldav">
    <D:prop xmlns:D="DAV:">
      <D:getetag/>
      <D:getcontenttype/>
      <D:getcontentlength/>
      <D:getlastmodified/>
      <C:calendar-data/>
    </D:prop>
    <C:filter>
        <C:comp-filter name="VCALENDAR">
          <C:comp-filter name="VEVENT"/>
        </C:comp-filter>
    </C:filter>
</C:calendar-query>
`

var propFindTest1 = `
<?xml version="1.0" encoding="UTF-8"?>                                                                 
<A:propfind xmlns:A="DAV:">
  <A:prop>
    <B:calendar-home-set xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <B:calendar-user-address-set xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <B:max-attendees-per-instance xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <A:principal-collection-set/>
    <A:principal-URL/>
    <A:resource-id/>
    <A:supported-report-set/>
    <B:supported-calendar-component-set xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <B:max-resource-size xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <B:calendar-timezone xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <A:current-user-principal/>
    <A:displayname/>    
    <B:calendar-description xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <B:calendar-data xmlns:B="urn:ietf:params:xml:ns:caldav"/>
    <A:resourcetype/>
    <A:getcontenttype/>
    <A:getetag/>
  </A:prop>
</A:propfind>
`

var calendarTestData1 = `
BEGIN:VCALENDAR
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
X-ABC-GUID:E1CX5Dr-0007ym-Hz@example.com
END:VEVENT
END:VCALENDAR
`

func TestPropFindAllPropAndQuery(t *testing.T) {
	calendar := Calendar{
		Description:           "This is a description which SHOULD NOT be returned in allprop",
		Path:                  "/user/calendars/default/",
		SupportedComponentSet: []string{"VEVENT", "VTODO"},
	}
	cal, err := ical.NewDecoder(strings.NewReader(calendarTestData1)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	object := CalendarObject{
		Path: "/user/calendars/default/DC6C50A017428C5216A2F1CD.ics",
		Data: cal,
		ETag: "191382932849",
	}
	handler := Handler{Backend: testBackend{
		calendars: []Calendar{calendar},
		objectMap: map[string][]CalendarObject{
			calendar.Path: []CalendarObject{object},
		},
	}}

	req := httptest.NewRequest("PROPFIND", "/user/calendars/default/", strings.NewReader(propFindAllProp))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp := string(data)
	if !strings.Contains(resp, "<resourcetype xmlns=\"DAV:\">") {
		t.Fatalf("want resourcetype prop in allprop")
	} else if !strings.Contains(resp, "<collection xmlns=\"DAV:\">") {
		t.Fatalf("want collection resourcetype")
	} else if !strings.Contains(resp, "<calendar xmlns=\"urn:ietf:params:xml:ns:caldav\">") {
		t.Fatalf("expect calendar resourcetype")
	} else if strings.Contains(resp, "<calendar-description xmlns=\"urn:ietf:params:xml:ns:caldav\">") {
		t.Fatalf("do not want calendar-description in allprop")
	} else if strings.Contains(resp, "DC6C50A017428C5216A2F1CD.ics") {
		t.Fatalf("do not want children if Depth: 0")
	}

	req = httptest.NewRequest("PROPFIND", "/user/calendars/default/", strings.NewReader(propFindAllProp))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp = string(data)
	if !strings.Contains(resp, fmt.Sprintf("<href>%s</href>", object.Path)) {
		t.Fatalf("want child href in allprop")
	} else if !strings.Contains(resp, object.ETag) {
		t.Fatalf("want child ETag in allprop")
	} else if !strings.Contains(resp, "<getcontenttype xmlns=\"DAV:\">text/calendar</getcontenttype>") {
		t.Fatalf("want child getcontenttype in allprop")
	} else if strings.Contains(resp, "<calendar-data xmlns=\"urn:ietf:params:xml:ns:caldav\">") {
		t.Fatalf("do not want calendar-data in allprop")
	}

	req = httptest.NewRequest("REPORT", "/user/calendars/default/", strings.NewReader(reportTest1))
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp = string(data)
	if !strings.Contains(resp, fmt.Sprintf("<href>%s</href>", object.Path)) {
		t.Fatalf("want child href in REPORT")
	} else if !strings.Contains(resp, object.ETag) {
		t.Fatalf("want child ETag in REPORT")
	} else if !strings.Contains(resp, "<getcontenttype xmlns=\"DAV:\">text/calendar</getcontenttype>") {
		t.Fatalf("want child getcontenttype in REPORT")
	} else if !strings.Contains(resp, "<calendar-data xmlns=\"urn:ietf:params:xml:ns:caldav\">") {
		t.Fatalf("do want calendar-data in REPORT")
	} else if !strings.Contains(resp, "UID:DC6C50A017428C5216A2F1CD@example.com") {
		t.Fatalf("calendar-data improperly returned")
	}

	req = httptest.NewRequest("PROPFIND", object.Path, strings.NewReader(propFindTest1))
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp = string(data)
	if strings.Contains(resp, "UID:DC6C50A017428C5216A2F1CD@example.com") {
		t.Fatalf("do not want calendar data in PROPFIND")
	} else if !strings.Contains(resp, object.ETag) {
		t.Fatalf("want child ETag in PROPFIND")
	} else if !strings.Contains(resp, "<getcontenttype xmlns=\"DAV:\">text/calendar</getcontenttype>") {
		t.Fatalf("want child getcontenttype in PROPFIND")
	}
}

var calendarTestData2 = `
BEGIN:VCALENDAR
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
END:VCALENDAR
`

var multigetTest1 = `
<?xml version="1.0" encoding="utf-8" ?>
   <C:calendar-multiget xmlns:D="DAV:"
                    xmlns:C="urn:ietf:params:xml:ns:caldav">
     <D:prop>
       <D:getetag/>
       <C:calendar-data/>
     </D:prop>
     <D:href>/user/calendars/default/74855313FA803DA593CD579A.ics</D:href>
     <D:href>/user/calendars/default/DC6C50A017428C5216A2F1CD.ics</D:href>
   </C:calendar-multiget>
`

func TestFindMultiget(t *testing.T) {
	calendar := Calendar{
		Description:           "This is a description which SHOULD NOT be returned in allprop",
		Path:                  "/user/calendars/default/",
		SupportedComponentSet: []string{"VEVENT"},
	}
	cal, err := ical.NewDecoder(strings.NewReader(calendarTestData1)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	object1 := CalendarObject{
		Path: "/user/calendars/default/DC6C50A017428C5216A2F1CD.ics",
		Data: cal,
		ETag: "191382932849",
	}
	cal, err = ical.NewDecoder(strings.NewReader(calendarTestData2)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	object2 := CalendarObject{
		Path: "/user/calendars/default/74855313FA803DA593CD579A.ics",
		Data: cal,
		ETag: "191382932850",
	}
	handler := Handler{Backend: testBackend{
		calendars: []Calendar{calendar},
		objectMap: map[string][]CalendarObject{
			calendar.Path: []CalendarObject{object1, object2},
		},
	}}

	req := httptest.NewRequest("REPORT", "/user/calendars/default/", strings.NewReader(multigetTest1))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp := string(data)
	if !strings.Contains(resp, "UID:DC6C50A017428C5216A2F1CD@example.com") {
		t.Fatalf("want object1 in multiget report")
	} else if !strings.Contains(resp, "UID:74855313FA803DA593CD579A@example.com") {
		t.Fatalf("want object2 in multiget report")
	} else if !strings.Contains(resp, object1.ETag) {
		t.Fatalf("want object1 ETag in multiget report")
	} else if !strings.Contains(resp, object2.ETag) {
		t.Fatalf("want object2 ETag in multiget report")
	}
}

type testBackend struct {
	calendars []Calendar
	objectMap map[string][]CalendarObject
}

func (t testBackend) CreateCalendar(ctx context.Context, calendar *Calendar) error {
	return nil
}

func (t testBackend) ListCalendars(ctx context.Context) ([]Calendar, error) {
	return t.calendars, nil
}

func (t testBackend) GetCalendar(ctx context.Context, path string) (*Calendar, error) {
	for _, cal := range t.calendars {
		if cal.Path == path {
			return &cal, nil
		}
	}
	return nil, fmt.Errorf("Calendar for path: %s not found", path)
}

func (t testBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/user/calendars/", nil
}

func (t testBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/user/", nil
}

func (t testBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	return nil
}

func (t testBackend) GetCalendarObject(ctx context.Context, path string, req *CalendarCompRequest) (*CalendarObject, error) {
	for _, objs := range t.objectMap {
		for _, obj := range objs {
			if obj.Path == path {
				return &obj, nil
			}
		}
	}
	return nil, fmt.Errorf("Couldn't find calendar object at: %s", path)
}

func (t testBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *PutCalendarObjectOptions) (*CalendarObject, error) {
	return nil, nil
}

func (t testBackend) ListCalendarObjects(ctx context.Context, path string, req *CalendarCompRequest) ([]CalendarObject, error) {
	return t.objectMap[path], nil
}

func (t testBackend) QueryCalendarObjects(ctx context.Context, path string, query *CalendarQuery) ([]CalendarObject, error) {
	if cos, err := t.ListCalendarObjects(ctx, path, nil); err != nil {
		return nil, err
	} else {
		return Filter(query, cos)
	}
}
