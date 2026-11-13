package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const findCalendarsReadOnlyResponse = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:response>
    <d:href>/cal/read-only/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><c:calendar/></d:resourcetype>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/cal/write-content/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><c:calendar/></d:resourcetype>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
          <d:privilege><d:write-content/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/cal/write/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><c:calendar/></d:resourcetype>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
          <d:privilege><d:write/></d:privilege>
          <d:privilege><d:write-content/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/cal/unknown/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><c:calendar/></d:resourcetype>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
    <d:propstat>
      <d:prop>
        <d:current-user-privilege-set/>
      </d:prop>
      <d:status>HTTP/1.1 404 Not Found</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestFindCalendarsReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(findCalendarsReadOnlyResponse))
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cals, err := c.FindCalendars(context.Background(), "/cal/")
	if err != nil {
		t.Fatalf("FindCalendars: %v", err)
	}

	want := map[string]bool{
		"/cal/read-only/":     true,
		"/cal/write-content/": true,
		"/cal/write/":         false,
		"/cal/unknown/":       false,
	}
	if len(cals) != len(want) {
		t.Fatalf("got %d calendars, want %d", len(cals), len(want))
	}
	for _, cal := range cals {
		if cal.ReadOnly != want[cal.Path] {
			t.Errorf("%s: ReadOnly = %v, want %v", cal.Path, cal.ReadOnly, want[cal.Path])
		}
	}
}
