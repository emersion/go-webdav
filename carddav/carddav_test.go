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

type testBackend struct {
	addressBooks []AddressBook
}

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

	currentUserPrincipalKey = contextKey("test:currentUserPrincipal")
	homeSetPathKey          = contextKey("test:homeSetPath")
	addressBookPathKey      = contextKey("test:addressBookPath")
)

func (*testBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	r := ctx.Value(currentUserPrincipalKey).(string)
	return r, nil
}

func (*testBackend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	r := ctx.Value(homeSetPathKey).(string)
	return r, nil
}

func (*testBackend) ListAddressBooks(ctx context.Context) ([]AddressBook, error) {
	p := ctx.Value(addressBookPathKey).(string)
	return []AddressBook{
		AddressBook{
			Path:                 p,
			Name:                 "My contacts",
			Description:          "Default address book",
			MaxResourceSize:      1024,
			SupportedAddressData: nil,
		},
	}, nil
}

func (b *testBackend) GetAddressBook(ctx context.Context, path string) (*AddressBook, error) {
	abs, err := b.ListAddressBooks(ctx)
	if err != nil {
		panic(err)
	}
	for _, ab := range abs {
		if ab.Path == path {
			return &ab, nil
		}
	}
	return nil, webdav.NewHTTPError(404, fmt.Errorf("Not found"))
}

func (b *testBackend) CreateAddressBook(ctx context.Context, ab *AddressBook) error {
	b.addressBooks = append(b.addressBooks, *ab)
	return nil
}

func (*testBackend) DeleteAddressBook(ctx context.Context, path string) error {
	panic("TODO: implement")
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

func (b *testBackend) ListAddressObjects(ctx context.Context, path string, req *AddressDataRequest) ([]AddressObject, error) {
	p := ctx.Value(addressBookPathKey).(string)
	if !strings.HasPrefix(path, p) {
		return nil, webdav.NewHTTPError(404, fmt.Errorf("Not found"))
	}
	alice, err := b.GetAddressObject(ctx, alicePath, req)
	if err != nil {
		return nil, err
	}

	return []AddressObject{*alice}, nil
}

func (*testBackend) QueryAddressObjects(ctx context.Context, path string, query *AddressBookQuery) ([]AddressObject, error) {
	panic("TODO: implement")
}

func (*testBackend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *PutAddressObjectOptions) (*AddressObject, error) {
	panic("TODO: implement")
}

func (*testBackend) DeleteAddressObject(ctx context.Context, path string) error {
	panic("TODO: implement")
}

func TestAddressBookDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		prefix               string
		currentUserPrincipal string
		homeSetPath          string
		addressBookPath      string
	}{
		{
			name:                 "simple",
			prefix:               "",
			currentUserPrincipal: "/test/",
			homeSetPath:          "/test/contacts/",
			addressBookPath:      "/test/contacts/private",
		},
		{
			name:                 "prefix",
			prefix:               "/dav",
			currentUserPrincipal: "/dav/test/",
			homeSetPath:          "/dav/test/contacts/",
			addressBookPath:      "/dav/test/contacts/private",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			h := Handler{&testBackend{}, tc.prefix}
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
			cup, err := client.FindCurrentUserPrincipal(ctx)
			if err != nil {
				t.Fatalf("error finding user principal url: %s", err)
			}
			if cup != tc.currentUserPrincipal {
				t.Fatalf("Found current user principal URL '%s', expected '%s'", cup, tc.currentUserPrincipal)
			}
			hsp, err := client.FindAddressBookHomeSet(ctx, cup)
			if err != nil {
				t.Fatalf("error finding home set path: %s", err)
			}
			if hsp != tc.homeSetPath {
				t.Fatalf("Found home set path '%s', expected '%s'", hsp, tc.homeSetPath)
			}
			abs, err := client.FindAddressBooks(ctx, hsp)
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

func TestRedirections(t *testing.T) {
	ctx := context.Background()

	h := Handler{&testBackend{}, ""}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, currentUserPrincipalKey, "/principal/")
		ctx = context.WithValue(ctx, homeSetPathKey, "/principal/contacts/")
		ctx = context.WithValue(ctx, addressBookPathKey, "/principal/contacts/default")
		r = r.WithContext(ctx)
		(&h).ServeHTTP(w, r)
	}))
	defer ts.Close()

	client, err := NewClient(nil, ts.URL)
	if err != nil {
		t.Fatalf("error creating client: %s", err)
	}
	hsp, err := client.FindAddressBookHomeSet(ctx, "/must-be-redirected/")
	if err != nil {
		t.Fatalf("error finding home set path: %s", err)
	}
	if want := "/principal/contacts/"; hsp != want {
		t.Fatalf("Found home set path '%s', expected '%s'", hsp, want)
	}
	abs, err := client.FindAddressBooks(ctx, "/must-be-redirected/again/")
	if err != nil {
		t.Fatalf("error finding address books: %s", err)
	}
	if len(abs) != 1 {
		t.Fatalf("Found %d address books, expected 1", len(abs))
	}
	if want := "/principal/contacts/default"; abs[0].Path != want {
		t.Fatalf("Found address book at %s, expected %s", abs[0].Path, want)
	}
}

var mkcolRequestBody = `
<?xml version="1.0" encoding="utf-8" ?>
   <D:mkcol xmlns:D="DAV:"
                 xmlns:C="urn:ietf:params:xml:ns:carddav">
     <D:set>
       <D:prop>
         <D:resourcetype>
           <D:collection/>
           <C:addressbook/>
         </D:resourcetype>
         <D:displayname>Lisa's Contacts</D:displayname>
         <C:addressbook-description xml:lang="en"
   >My primary address book.</C:addressbook-description>
       </D:prop>
     </D:set>
   </D:mkcol>`

func TestCreateAddressbookMinimalBody(t *testing.T) {
	tb := testBackend{
		addressBooks: nil,
	}
	b := backend{
		Backend: &tb,
		Prefix:  "/dav",
	}
	req := httptest.NewRequest("MKCOL", "/dav/addressbooks/user0/test-addressbook", strings.NewReader(mkcolRequestBody))
	req.Header.Set("Content-Type", "application/xml")

	err := b.Mkcol(req)
	if err != nil {
		t.Fatalf("Unexpcted error in Mkcol: %s", err)
	}
	if len(tb.addressBooks) != 1 {
		t.Fatalf("Found %d address books, expected 1", len(tb.addressBooks))
	}
	c := tb.addressBooks[0]
	if c.Name != "Lisa's Contacts" {
		t.Fatalf("Address book name is '%s', expected 'Lisa's Contacts'", c.Name)
	}
	if c.Path != "/dav/addressbooks/user0/test-addressbook" {
		t.Fatalf("Address book path is '%s', expected '/dav/addressbooks/user0/test-addressbook'", c.Path)
	}
	if c.Description != "My primary address book." {
		t.Fatalf("Address book sdscription is '%s', expected 'My primary address book.'", c.Description)
	}
}
