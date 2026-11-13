package carddav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const findAddressBooksReadOnlyResponse = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:response>
    <d:href>/contacts/read-only/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><card:addressbook/></d:resourcetype>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/contacts/all/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><card:addressbook/></d:resourcetype>
        <d:current-user-privilege-set>
          <d:privilege><d:all/></d:privilege>
          <d:privilege><d:read/></d:privilege>
          <d:privilege><d:write/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestFindAddressBooksReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(findAddressBooksReadOnlyResponse))
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	abs, err := c.FindAddressBooks(context.Background(), "/contacts/")
	if err != nil {
		t.Fatalf("FindAddressBooks: %v", err)
	}

	want := map[string]bool{
		"/contacts/read-only/": true,
		"/contacts/all/":       false,
	}
	if len(abs) != len(want) {
		t.Fatalf("got %d address books, want %d", len(abs), len(want))
	}
	for _, ab := range abs {
		if ab.ReadOnly != want[ab.Path] {
			t.Errorf("%s: ReadOnly = %v, want %v", ab.Path, ab.ReadOnly, want[ab.Path])
		}
	}
}
