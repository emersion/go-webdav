package caldav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/internal"
)

// syncBackend serves one calendar whose single event is deleted after
// token "1".
type syncBackend struct {
	testBackend
}

func (b syncBackend) SyncCalendarObjects(ctx context.Context, path string, query *SyncQuery) (*SyncResponse, error) {
	switch query.SyncToken {
	case "":
		return &SyncResponse{SyncToken: "1", Updated: b.objectMap[path]}, nil
	case "1":
		return &SyncResponse{SyncToken: "2", Deleted: []string{b.objectMap[path][0].Path}}, nil
	}
	return nil, ErrInvalidSyncToken
}

func newSyncBackend(t *testing.T) syncBackend {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//test//test")
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "event")
	event.Props.SetText(ical.PropSummary, "Event")
	event.Props.SetText(ical.PropDateTimeStamp, "20260101T000000Z")
	event.Props.SetText(ical.PropDateTimeStart, "20260101T100000Z")
	cal.Children = append(cal.Children, event.Component)
	return syncBackend{testBackend{
		calendars: []Calendar{{Path: "/user/calendars/cal/", SyncToken: "1"}},
		objectMap: map[string][]CalendarObject{
			"/user/calendars/cal/": {{Path: "/user/calendars/cal/event.ics", ETag: "e1", Data: cal}},
		},
	}}
}

func TestSyncCollection(t *testing.T) {
	srv := httptest.NewServer(&Handler{Backend: newSyncBackend(t)})
	defer srv.Close()
	c, err := NewClient(nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := c.SyncCollection(ctx, "/user/calendars/cal/", &SyncQuery{CompRequest: CalendarCompRequest{AllProps: true, AllComps: true}})
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if res.SyncToken != "1" || len(res.Updated) != 1 || res.Updated[0].Path != "/user/calendars/cal/event.ics" || len(res.Deleted) != 0 {
		t.Fatalf("initial sync: %+v", res)
	}

	res, err = c.SyncCollection(ctx, "/user/calendars/cal/", &SyncQuery{SyncToken: "1"})
	if err != nil {
		t.Fatalf("delta sync: %v", err)
	}
	if res.SyncToken != "2" || len(res.Updated) != 0 || len(res.Deleted) != 1 || res.Deleted[0] != "/user/calendars/cal/event.ics" {
		t.Fatalf("delta sync: %+v", res)
	}

	_, err = c.SyncCollection(ctx, "/user/calendars/cal/", &SyncQuery{SyncToken: "bogus"})
	var httpErr *internal.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Fatalf("unknown token: %v", err)
	}
}

func TestSyncCollectionUnsupported(t *testing.T) {
	srv := httptest.NewServer(&Handler{Backend: newSyncBackend(t).testBackend})
	defer srv.Close()
	c, err := NewClient(nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SyncCollection(context.Background(), "/user/calendars/cal/", &SyncQuery{}); err == nil {
		t.Fatal("sync-collection on a backend without SyncBackend succeeded")
	}
}

func TestPropFindSyncToken(t *testing.T) {
	h := Handler{Backend: newSyncBackend(t)}
	req := httptest.NewRequest("PROPFIND", "/user/calendars/cal/", strings.NewReader(`<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/">
  <d:prop><d:sync-token/><cs:getctag/><d:supported-report-set/></d:prop>
</d:propfind>`))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	body, _ := io.ReadAll(w.Result().Body)
	for _, want := range []string{`<sync-token xmlns="DAV:">1</sync-token>`, `<getctag xmlns="http://calendarserver.org/ns/">1</getctag>`, `<sync-collection xmlns="DAV:">`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("PROPFIND lacks %q:\n%s", want, body)
		}
	}
}
