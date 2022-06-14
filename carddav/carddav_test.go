package carddav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
)

type testBackend struct{}

type contextKey string

var (
	aliceData = `BEGIN:VCARD
VERSION:4.0
UID:urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b1
FN;PID=1.1:Alice Gopher
N:Gopher;Alice;;;
EMAIL;PID=1.1:alice@example.com
CLIENTPIDMAP:1;urn:uuid:53e374d9-337e-4727-8803-a1e9c14e0551
END:VCARD`
	alicePath = "urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b1.vcf"

	currentUserPrincipalKey = "test:currentUserPrincipal"
	homeSetPathKey          = "test:homeSetPath"
	addressBookPathKey      = "test:addressBookPath"
)

func (*testBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	r := ctx.Value(currentUserPrincipalKey).(string)
	return r, nil
}

func (*testBackend) AddressbookHomeSetPath(ctx context.Context) (string, error) {
	r := ctx.Value(homeSetPathKey).(string)
	return r, nil
}

func (*testBackend) AddressBook(ctx context.Context) (*AddressBook, error) {
	p := ctx.Value(addressBookPathKey).(string)
	return &AddressBook{
		Path:                 p,
		Name:                 "My contacts",
		Description:          "Default address book",
		MaxResourceSize:      1024,
		SupportedAddressData: nil,
	}, nil
}

func (*testBackend) GetAddressObject(ctx context.Context, path string, req *AddressDataRequest) (*AddressObject, error) {
	if path == alicePath {
		card, err := vcard.NewDecoder(strings.NewReader(aliceData)).Decode()
		if err != nil {
			return nil, err
		}
		return &AddressObject{
			Path: path,
			Card: card,
		}, nil
	} else {
		return nil, webdav.NewHTTPError(404, fmt.Errorf("Not found"))
	}
}

func (b *testBackend) ListAddressObjects(ctx context.Context, req *AddressDataRequest) ([]AddressObject, error) {
	alice, err := b.GetAddressObject(ctx, alicePath, req)
	if err != nil {
		return nil, err
	}

	return []AddressObject{*alice}, nil
}

func (*testBackend) QueryAddressObjects(ctx context.Context, query *AddressBookQuery) ([]AddressObject, error) {
	panic("TODO: implement")
}

func (*testBackend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *PutAddressObjectOptions) (loc string, err error) {
	panic("TODO: implement")
}

func (*testBackend) DeleteAddressObject(ctx context.Context, path string) error {
	panic("TODO: implement")
}

func TestAddressBookDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		currentUserPrincipal string
		homeSetPath          string
		addressBookPath      string
	}{
		// TODO this used to work, but is currently broken.
		//{
		//	name:  "all-at-root",
		//	currentUserPrincipal: "/",
		//	homeSetPath: "/",
		//	addressBookPath: "/",
		//},
		{
			name:                 "simple-home-set-path",
			currentUserPrincipal: "/",
			homeSetPath:          "/contacts/",
			addressBookPath:      "/contacts/",
		},
		{
			name:                 "all-at-different-paths",
			currentUserPrincipal: "/",
			homeSetPath:          "/contacts/",
			addressBookPath:      "/contacts/work",
		},
		{
			name:                 "nothing-at-root",
			currentUserPrincipal: "/test/",
			homeSetPath:          "/test/contacts/",
			addressBookPath:      "/test/contacts/private",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			h := Handler{&testBackend{}}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				ctx = context.WithValue(ctx, currentUserPrincipalKey, tc.currentUserPrincipal)
				ctx = context.WithValue(ctx, homeSetPathKey, tc.homeSetPath)
				ctx = context.WithValue(ctx, addressBookPathKey, tc.addressBookPath)
				r = r.WithContext(ctx)
				(&h).ServeHTTP(w, r)
			}))
			defer ts.Close()

			// client supports .well-known discovery if explicitly pointed to it
			startURL := ts.URL
			if tc.currentUserPrincipal != "/" {
				startURL = ts.URL + "/.well-known/carddav"
			}

			client, err := NewClient(nil, startURL)
			if err != nil {
				t.Fatalf("error creating client: %s", err)
			}
			cup, err := client.FindCurrentUserPrincipal()
			if err != nil {
				t.Fatalf("error finding user principal url: %s", err)
			}
			if cup != tc.currentUserPrincipal {
				t.Fatalf("Found current user principal URL '%s', expected '%s'", cup, tc.currentUserPrincipal)
			}
			hsp, err := client.FindAddressBookHomeSet(cup)
			if err != nil {
				t.Fatalf("error finding home set path: %s", err)
			}
			if hsp != tc.homeSetPath {
				t.Fatalf("Found home set path '%s', expected '%s'", hsp, tc.homeSetPath)
			}
			abs, err := client.FindAddressBooks(hsp)
			if err != nil {
				t.Fatalf("error finding address books: %s", err)
			}
			if len(abs) != 1 {
				t.Fatalf("Found %d address books, expected 1", len(abs))
			}
			if abs[0].Path != tc.addressBookPath {
				t.Fatalf("Found address book at %s, expected %s", abs[0].Path, tc.addressBookPath)
			}
		})
	}
}
