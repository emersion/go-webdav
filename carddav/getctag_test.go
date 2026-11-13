package carddav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-webdav"
)

const ctagAddressBookPath = "/test/contacts/private"

// ctagBackend serves a single address book with a caller-supplied CTag. The
// shared testBackend synthesises its address book from the request context, so
// it cannot carry one.
type ctagBackend struct {
	*testBackend
	ctag string
}

func (b *ctagBackend) ListAddressBooks(ctx context.Context) ([]AddressBook, error) {
	return []AddressBook{{
		Path: ctagAddressBookPath,
		Name: "Private",
		CTag: b.ctag,
	}}, nil
}

func (b *ctagBackend) GetAddressBook(ctx context.Context, path string) (*AddressBook, error) {
	abs, err := b.ListAddressBooks(ctx)
	if err != nil {
		return nil, err
	}
	for _, ab := range abs {
		if ab.Path == path {
			return &ab, nil
		}
	}
	return nil, webdav.NewHTTPError(404, fmt.Errorf("not found"))
}

// propFindCTag issues the Depth: 0 PROPFIND that Apple clients use to decide
// whether a collection needs enumerating, and returns the response body.
func propFindCTag(t *testing.T, ctag string) string {
	t.Helper()

	h := Handler{&ctagBackend{testBackend: &testBackend{}, ctag: ctag}, ""}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, currentUserPrincipalKey, "/test/")
		ctx = context.WithValue(ctx, homeSetPathKey, "/test/contacts/")
		ctx = context.WithValue(ctx, addressBookPathKey, ctagAddressBookPath)
		(&h).ServeHTTP(w, r.WithContext(ctx))
	}))
	defer ts.Close()

	body := `<?xml version="1.0" encoding="UTF-8"?>
<A:propfind xmlns:A="DAV:" xmlns:C="http://calendarserver.org/ns/">
  <A:prop><A:resourcetype/><C:getctag/></A:prop>
</A:propfind>`

	req, err := http.NewRequest("PROPFIND", ts.URL+ctagAddressBookPath, strings.NewReader(body))
	if err != nil {
		t.Fatalf("error building request: %s", err)
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error performing PROPFIND: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("PROPFIND returned %d, expected 207", resp.StatusCode)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading body: %s", err)
	}
	return string(out)
}

func TestAddressBookCTagServedWhenSet(t *testing.T) {
	body := propFindCTag(t, "abc123")

	if !strings.Contains(body, `<getctag xmlns="http://calendarserver.org/ns/">abc123</getctag>`) {
		t.Errorf("CS:getctag not served with the backend's value:\n%s", body)
	}
	if strings.Contains(body, "404") {
		t.Errorf("requested properties should all resolve, got a 404 propstat:\n%s", body)
	}
}

func TestAddressBookCTagNotFoundWhenUnset(t *testing.T) {
	body := propFindCTag(t, "")

	// A backend that supplies no CTag must not fabricate one; the requested
	// property is reported not found, as WebDAV requires.
	if strings.Contains(body, "</getctag>") && !strings.Contains(body, "<getctag") {
		t.Fatalf("unexpected getctag serialisation:\n%s", body)
	}
	if !strings.Contains(body, "404 Not Found") {
		t.Errorf("unset CTag should be reported in a 404 propstat:\n%s", body)
	}
}
