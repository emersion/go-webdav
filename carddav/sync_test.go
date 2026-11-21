package carddav

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/internal"
)

const syncBookPath = "/dav/contacts/private/"

// syncBackend serves one address book whose single card is deleted after
// token "1".
type syncBackend struct {
	testBackend
}

func (b *syncBackend) SyncAddressObjects(ctx context.Context, path string, query *SyncQuery) (*SyncResponse, error) {
	switch query.SyncToken {
	case "":
		card, err := vcard.NewDecoder(strings.NewReader(aliceData)).Decode()
		if err != nil {
			return nil, err
		}
		return &SyncResponse{SyncToken: "1", Updated: []AddressObject{{Path: syncBookPath + alicePath, ETag: "a1", Card: card}}}, nil
	case "1":
		return &SyncResponse{SyncToken: "2", Deleted: []string{syncBookPath + alicePath}}, nil
	}
	return nil, ErrInvalidSyncToken
}

func TestSyncCollection(t *testing.T) {
	h := Handler{Backend: &syncBackend{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), currentUserPrincipalKey, "/dav/")
		ctx = context.WithValue(ctx, homeSetPathKey, "/dav/contacts/")
		ctx = context.WithValue(ctx, addressBookPathKey, syncBookPath)
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()
	c, err := NewClient(nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := c.SyncCollection(ctx, syncBookPath, &SyncQuery{DataRequest: AddressDataRequest{AllProp: true}})
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if res.SyncToken != "1" || len(res.Updated) != 1 || res.Updated[0].Path != syncBookPath+alicePath || len(res.Deleted) != 0 {
		t.Fatalf("initial sync: %+v", res)
	}
	if res.Updated[0].ETag != "a1" {
		t.Errorf("etag: %q", res.Updated[0].ETag)
	}

	res, err = c.SyncCollection(ctx, syncBookPath, &SyncQuery{SyncToken: "1"})
	if err != nil {
		t.Fatalf("delta sync: %v", err)
	}
	if res.SyncToken != "2" || len(res.Updated) != 0 || len(res.Deleted) != 1 || res.Deleted[0] != syncBookPath+alicePath {
		t.Fatalf("delta sync: %+v", res)
	}

	_, err = c.SyncCollection(ctx, syncBookPath, &SyncQuery{SyncToken: "bogus"})
	var httpErr *internal.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Fatalf("unknown token: %v", err)
	}
}
