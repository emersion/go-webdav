package caldav

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersion/go-webdav/internal"
)

const queryCalendarMalformedResponse = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:response>
    <d:href>/cal/good.ics</d:href>
    <d:propstat>
      <d:prop>
        <d:getetag>"good"</d:getetag>
        <c:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-webdav//test//EN
BEGIN:VEVENT
UID:good
DTSTAMP:20260624T000000Z
END:VEVENT
END:VCALENDAR
</c:calendar-data>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/cal/bad.ics</d:href>
    <d:propstat>
      <d:prop>
        <d:getetag>"bad"</d:getetag>
        <c:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:bad
</c:calendar-data>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestQueryCalendarMalformedObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(queryCalendarMalformedResponse))
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	objs, err := c.QueryCalendar(context.Background(), "/cal/", &CalendarQuery{
		CompFilter: CompFilter{Name: "VCALENDAR"},
	})

	var hrefErr *internal.HrefError
	if !errors.As(err, &hrefErr) {
		t.Fatalf("QueryCalendar: want an *internal.HrefError, got %v", err)
	}
	if hrefErr.Href.Path != "/cal/bad.ics" {
		t.Errorf("HrefError.Href = %q, want %q", hrefErr.Href.Path, "/cal/bad.ics")
	}
	if len(objs) != 1 {
		t.Fatalf("got %d objects, want 1", len(objs))
	}
	if objs[0].Path != "/cal/good.ics" || objs[0].ETag != "good" || objs[0].Data == nil {
		t.Errorf("got object %+v, want /cal/good.ics with ETag good and data", objs[0])
	}
}
