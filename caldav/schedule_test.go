package caldav

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
)

type scheduleTestBackend struct {
	testBackend
	got *ical.Calendar
}

func (b *scheduleTestBackend) ScheduleOutboxPath(context.Context) (string, error) {
	return "/user/outbox/", nil
}

func (b *scheduleTestBackend) CalendarUserAddresses(context.Context) ([]string, error) {
	return []string{"mailto:user@example.com"}, nil
}

func (b *scheduleTestBackend) FreeBusy(_ context.Context, req *ical.Calendar) ([]ScheduleResponse, error) {
	b.got = req
	reply := ical.NewCalendar()
	reply.Props.SetText(ical.PropVersion, "2.0")
	reply.Props.SetText(ical.PropProductID, "-//t//t")
	reply.Props.SetText(ical.PropMethod, "REPLY")
	fb := ical.NewComponent(ical.CompFreeBusy)
	fb.Props.SetText(ical.PropUID, "fb1")
	fb.Props.Set(&ical.Prop{Name: ical.PropDateTimeStamp, Value: "20261101T000000Z", Params: ical.Params{}})
	fb.Props.Set(&ical.Prop{Name: "FREEBUSY", Value: "20261201T090000Z/20261201T100000Z", Params: ical.Params{}})
	reply.Children = append(reply.Children, fb)
	return []ScheduleResponse{
		{Recipient: "mailto:jan@example.com", RequestStatus: "2.0;Success", Calendar: reply},
		{Recipient: "mailto:far@example.org", RequestStatus: "3.7;Invalid calendar user"},
	}, nil
}

// A ScheduleBackend's principal names its outbox and addresses, and the
// outbox answers a VFREEBUSY request with a schedule-response; a plain
// Backend's principal carries neither.
func TestScheduleOutbox(t *testing.T) {
	b := &scheduleTestBackend{}
	h := &Handler{Backend: b}

	req := httptest.NewRequest("PROPFIND", "/user/", strings.NewReader(`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:schedule-outbox-URL/><c:calendar-user-address-set/></d:prop></d:propfind>`))
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, ">/user/outbox/</href>") || !strings.Contains(body, ">mailto:user@example.com</href>") {
		t.Fatalf("principal: %d %s", w.Code, body)
	}

	plain := &Handler{Backend: testBackend{}}
	req = httptest.NewRequest("PROPFIND", "/user/", strings.NewReader(`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:schedule-outbox-URL/></d:prop></d:propfind>`))
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	plain.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "/user/outbox/") || !strings.Contains(w.Body.String(), "404") {
		t.Fatalf("plain principal: %s", w.Body.String())
	}

	request := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t\r\nMETHOD:REQUEST\r\nBEGIN:VFREEBUSY\r\nUID:fb1\r\nDTSTAMP:20261101T000000Z\r\nDTSTART:20261201T000000Z\r\nDTEND:20261202T000000Z\r\nORGANIZER:mailto:user@example.com\r\nATTENDEE:mailto:jan@example.com\r\nATTENDEE:mailto:far@example.org\r\nEND:VFREEBUSY\r\nEND:VCALENDAR\r\n"
	req = httptest.NewRequest("POST", "/user/outbox/", strings.NewReader(request))
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	out, _ := io.ReadAll(w.Body)
	for _, want := range []string{"schedule-response", `<href xmlns="DAV:">mailto:jan@example.com</href></recipient>`, "2.0;Success", "FREEBUSY:20261201T090000Z/20261201T100000Z", "3.7;Invalid calendar user"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("POST: %d, %q missing in %s", w.Code, want, out)
		}
	}
	if b.got == nil || b.got.Children[0].Props.Get(ical.PropUID).Value != "fb1" {
		t.Fatalf("request reached the backend as %+v", b.got)
	}

	// An event POSTed to the outbox is refused: the server does not schedule.
	req = httptest.NewRequest("POST", "/user/outbox/", strings.NewReader(strings.ReplaceAll(request, "VFREEBUSY", "VEVENT")))
	req.Header.Set("Content-Type", "text/calendar")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("an event: %d", w.Code)
	}
}
