package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
)

// minimalCalendar builds a single-VEVENT VCALENDAR valid enough to encode.
func minimalCalendar() *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//go-webdav//test//EN")
	ev := ical.NewEvent()
	ev.Props.SetText(ical.PropUID, "test-uid")
	ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, ev.Component)
	return cal
}

// TestPutCalendarObjectSendsIfMatch verifies the client forwards the
// PutCalendarObjectOptions.IfMatch ETag as an If-Match request header and
// returns the server's new ETag.
func TestPutCalendarObjectSendsIfMatch(t *testing.T) {
	var gotIfMatch string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotIfMatch = r.Header.Get("If-Match")
		w.Header().Set("ETag", `"new-etag"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	opts := &PutCalendarObjectOptions{IfMatch: webdav.ConditionalMatch(`"old-etag"`)}
	co, err := c.PutCalendarObject(context.Background(), "/cal/test-uid.ics", minimalCalendar(), opts)
	if err != nil {
		t.Fatalf("PutCalendarObject: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotIfMatch != `"old-etag"` {
		t.Errorf("If-Match header = %q, want %q", gotIfMatch, `"old-etag"`)
	}
	if co.ETag != "new-etag" {
		t.Errorf("returned ETag = %q, want %q", co.ETag, "new-etag")
	}
}

// TestPutCalendarObjectNilOptsOmitsIfMatch verifies a nil opts (the
// unconditional write) sends no If-Match header.
func TestPutCalendarObjectNilOptsOmitsIfMatch(t *testing.T) {
	var hadIfMatch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadIfMatch = r.Header["If-Match"]
		w.Header().Set("ETag", `"e"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.PutCalendarObject(context.Background(), "/cal/test-uid.ics", minimalCalendar(), nil); err != nil {
		t.Fatalf("PutCalendarObject: %v", err)
	}
	if hadIfMatch {
		t.Errorf("If-Match header present on unconditional PUT, want absent")
	}
}

// TestPutCalendarObjectPreconditionFailed verifies a 412 response surfaces as
// an error whose status code consumers can read via webdav.HTTPErrorCode.
func TestPutCalendarObjectPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	opts := &PutCalendarObjectOptions{IfMatch: webdav.ConditionalMatch(`"stale"`)}
	_, err = c.PutCalendarObject(context.Background(), "/cal/test-uid.ics", minimalCalendar(), opts)
	if err == nil {
		t.Fatal("PutCalendarObject: want error on 412, got nil")
	}
	code, ok := webdav.HTTPErrorCode(err)
	if !ok {
		t.Fatalf("HTTPErrorCode: not an HTTP error: %v", err)
	}
	if code != http.StatusPreconditionFailed {
		t.Errorf("status code = %d, want %d", code, http.StatusPreconditionFailed)
	}
}
