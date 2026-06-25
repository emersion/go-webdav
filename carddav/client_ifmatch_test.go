package carddav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
)

func minimalCard() vcard.Card {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Alice Gopher")
	card.SetValue(vcard.FieldUID, "test-uid")
	vcard.ToV4(card)
	return card
}

// TestPutAddressObjectSendsIfMatch verifies the client forwards
// PutAddressObjectOptions.IfMatch as an If-Match request header and returns the
// server's new ETag.
func TestPutAddressObjectSendsIfMatch(t *testing.T) {
	var gotIfMatch, gotMethod string
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
	opts := &PutAddressObjectOptions{IfMatch: webdav.ConditionalMatch(`"old-etag"`)}
	ao, err := c.PutAddressObject(context.Background(), "/contacts/test-uid.vcf", minimalCard(), opts)
	if err != nil {
		t.Fatalf("PutAddressObject: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotIfMatch != `"old-etag"` {
		t.Errorf("If-Match header = %q, want %q", gotIfMatch, `"old-etag"`)
	}
	if ao.ETag != "new-etag" {
		t.Errorf("returned ETag = %q, want %q", ao.ETag, "new-etag")
	}
}

// TestPutAddressObjectPreconditionFailed verifies a 412 response surfaces as an
// error readable via webdav.HTTPErrorCode.
func TestPutAddressObjectPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	c, err := NewClient(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	opts := &PutAddressObjectOptions{IfMatch: webdav.ConditionalMatch(`"stale"`)}
	_, err = c.PutAddressObject(context.Background(), "/contacts/test-uid.vcf", minimalCard(), opts)
	if err == nil {
		t.Fatal("PutAddressObject: want error on 412, got nil")
	}
	code, ok := webdav.HTTPErrorCode(err)
	if !ok {
		t.Fatalf("HTTPErrorCode: not an HTTP error: %v", err)
	}
	if code != http.StatusPreconditionFailed {
		t.Errorf("status code = %d, want %d", code, http.StatusPreconditionFailed)
	}
}
