package caldav

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
		handler := Handler{Backend: &testBackend{calendars: []Calendar{*calendar}}}
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
	handler := Handler{Backend: &testBackend{calendars: []Calendar{*calendar}}}
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

const TestMkCalendarReq = `
<?xml version="1.0" encoding="UTF-8"?>
<B:mkcalendar xmlns:B="urn:ietf:params:xml:ns:caldav">
  <A:set xmlns:A="DAV:">
    <A:prop>
      <B:calendar-timezone>BEGIN:VCALENDAR&#13;
VERSION:2.0&#13;
PRODID:-//Apple Inc.//iPhone OS 18.1.1//EN&#13;
CALSCALE:GREGORIAN&#13;
BEGIN:VTIMEZONE&#13;
TZID:Europe/Paris&#13;
BEGIN:DAYLIGHT&#13;
TZOFFSETFROM:+0100&#13;
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU&#13;
DTSTART:19810329T020000&#13;
TZNAME:UTC+2&#13;
TZOFFSETTO:+0200&#13;
END:DAYLIGHT&#13;
BEGIN:STANDARD&#13;
TZOFFSETFROM:+0200&#13;
RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU&#13;
DTSTART:19961027T030000&#13;
TZNAME:UTC+1&#13;
TZOFFSETTO:+0100&#13;
END:STANDARD&#13;
END:VTIMEZONE&#13;
END:VCALENDAR&#13;
</B:calendar-timezone>
      <D:calendar-order xmlns:D="http://apple.com/ns/ical/">2</D:calendar-order>
      <B:supported-calendar-component-set>
        <B:comp name="VEVENT"/>
      </B:supported-calendar-component-set>
      <D:calendar-color xmlns:D="http://apple.com/ns/ical/" symbolic-color="red">#FF2968</D:calendar-color>
      <A:displayname>test calendar</A:displayname>
      <B:calendar-free-busy-set>
        <NO/>
      </B:calendar-free-busy-set>
    </A:prop>
  </A:set>
</B:mkcalendar>
`

const propFindTest2 = `
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
     <d:resourcetype/>
     <c:supported-calendar-component-set/>
     <d:displayname/>
     <c:max-resource-size/>
     <c:calendar-description/>
  </d:prop>
</d:propfind>
`

func TestMkCalendar(t *testing.T) {
	handler := Handler{Backend: &testBackend{
		calendars: []Calendar{},
		objectMap: map[string][]CalendarObject{},
	}}

	req := httptest.NewRequest("MKCALENDAR", "/user/calendars/default/", strings.NewReader(TestMkCalendarReq))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res := w.Result()
	if e := res.Body.Close(); e != nil {
		t.Fatal(e)
	} else if loc := res.Header.Get("Location"); loc != "/user/calendars/default/" {
		t.Fatalf("unexpected location: %s", loc)
	} else if sc := res.StatusCode; sc != http.StatusCreated {
		t.Fatalf("unexpected status code: %d", sc)
	}

	req = httptest.NewRequest("PROPFIND", "/user/calendars/default/", strings.NewReader(propFindTest2))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res = w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp := string(data)
	if !strings.Contains(resp, fmt.Sprintf("<href>%s</href>", "/user/calendars/default/")) {
		t.Fatalf("want calendar href in response")
	} else if !strings.Contains(resp, "<resourcetype xmlns=\"DAV:\">") {
		t.Fatalf("want resource type in response")
	} else if !strings.Contains(resp, "<collection xmlns=\"DAV:\"></collection>") {
		t.Fatalf("want collection resource type in response")
	} else if !strings.Contains(resp, "<calendar xmlns=\"urn:ietf:params:xml:ns:caldav\"></calendar>") {
		t.Fatalf("want calendar resource type in response")
	} else if !strings.Contains(resp, "<displayname xmlns=\"DAV:\">test calendar</displayname>") {
		t.Fatalf("want display name in response")
	} else if !strings.Contains(resp, "<supported-calendar-component-set xmlns=\"urn:ietf:params:xml:ns:caldav\"><comp xmlns=\"urn:ietf:params:xml:ns:caldav\" name=\"VEVENT\"></comp></supported-calendar-component-set>") {
		t.Fatalf("want supported-calendar-component-set in response")
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

type testBackend struct {
	calendars []Calendar
	objectMap map[string][]CalendarObject
}

func (t *testBackend) CreateCalendar(ctx context.Context, calendar *Calendar) error {
	if v, e := t.CalendarHomeSetPath(ctx); e != nil {
		return e
	} else if !strings.HasPrefix(calendar.Path, v) || len(calendar.Path) == len(v) {
		return fmt.Errorf("cannot create calendar at location %s", calendar.Path)
	} else {
		t.calendars = append(t.calendars, *calendar)
		return nil
	}
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

func (t *testBackend) DeleteCalendarObject(ctx context.Context, path string) error {
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

func (t *testBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *PutCalendarObjectOptions) (*CalendarObject, error) {
	return nil, nil
}

func (t testBackend) ListCalendarObjects(ctx context.Context, path string, req *CalendarCompRequest) ([]CalendarObject, error) {
	return t.objectMap[path], nil
}

func (t testBackend) QueryCalendarObjects(ctx context.Context, path string, query *CalendarQuery) ([]CalendarObject, error) {
	return nil, nil
}
