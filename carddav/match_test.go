package carddav

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

func TestFilter(t *testing.T) {
	newAO := func(str string) AddressObject {
		card, err := vcard.NewDecoder(strings.NewReader(str)).Decode()
		if err != nil {
			t.Fatal(err)
		}
		return AddressObject{
			Card: card,
		}
	}

	alice := newAO(`BEGIN:VCARD
VERSION:4.0
UID:urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b1
FN;PID=1.1:Alice Gopher
N:Gopher;Alice;;;
EMAIL;PID=1.1:alice@example.com
CLIENTPIDMAP:1;urn:uuid:53e374d9-337e-4727-8803-a1e9c14e0551
END:VCARD`)

	bob := newAO(`BEGIN:VCARD
VERSION:4.0
UID:urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b2
FN;PID=1.1:Bob Gopher
N:Gopher;Bob;;;
EMAIL;PID=1.1:bob@example.com
CLIENTPIDMAP:1;urn:uuid:53e374d9-337e-4727-8803-a1e9c14e0552
END:VCARD`)

	carla := newAO(`BEGIN:VCARD
VERSION:4.0
UID:urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b3
FN;PID=1.1:Carla Gopher
N:Gopher;Carla;;;
EMAIL;PID=1.1:carla@example.com
CLIENTPIDMAP:1;urn:uuid:53e374d9-337e-4727-8803-a1e9c14e0553
END:VCARD`)

	for _, tc := range []struct {
		name  string
		query *AddressBookQuery
		addrs []AddressObject
		want  []AddressObject
		err   error
	}{
		{
			name:  "nil-query",
			query: nil,
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice, bob, carla},
		},
		{
			name: "no-limit-query",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice, bob, carla},
		},
		{
			name: "limit-1-query",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				Limit: 1,
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice},
		},
		{
			name: "limit-4-query",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				Limit: 4,
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice, bob, carla},
		},
		{
			name: "email-match",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "carla"}},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{carla},
		},
		{
			name: "email-match-any",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{
							{Text: "carla@example"},
							{Text: "alice@example"},
						},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice, carla},
		},
		{
			name: "email-match-all",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{{
					Name: vcard.FieldEmail,
					TextMatches: []TextMatch{
						{Text: ""},
					},
				}},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{alice, bob, carla},
		},
		{
			name: "email-no-match",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.org"}},
					},
				},
			},
			addrs: []AddressObject{alice, bob, carla},
			want:  []AddressObject{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Filter(tc.query, tc.addrs)
			switch {
			case err != nil && tc.err == nil:
				t.Fatalf("unexpected error: %+v", err)
			case err != nil && tc.err != nil:
				if got, want := err.Error(), tc.err.Error(); got != want {
					t.Fatalf("invalid error:\ngot= %q\nwant=%q", got, want)
				}
			case err == nil && tc.err != nil:
				t.Fatalf("expected an error:\ngot= %+v\nwant=%+v", err, tc.err)
			case err == nil && tc.err == nil:
				if got, want := got, tc.want; !reflect.DeepEqual(got, want) {
					t.Fatalf("invalid filter values:\ngot= %+v\nwant=%+v", got, want)
				}
			}
		})
	}
}

func TestMatch(t *testing.T) {
	newAO := func(str string) AddressObject {
		card, err := vcard.NewDecoder(strings.NewReader(str)).Decode()
		if err != nil {
			t.Fatal(err)
		}
		return AddressObject{
			Card: card,
		}
	}

	alice := newAO(`BEGIN:VCARD
VERSION:4.0
UID:urn:uuid:4fbe8971-0bc3-424c-9c26-36c3e1eff6b1
FN;PID=1.1:Alice Gopher
N:Gopher;Alice;;;
EMAIL;PID=1.1:alice@example.com
CLIENTPIDMAP:1;urn:uuid:53e374d9-337e-4727-8803-a1e9c14e0556
END:VCARD`)

	for _, tc := range []struct {
		name  string
		query *AddressBookQuery
		addr  AddressObject
		want  bool
		err   error
	}{
		{
			name:  "nil-query",
			query: nil,
			addr:  alice,
			want:  true,
		},
		{
			name: "match-email-contains",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-email-equals-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:      "alice@example.com",
							MatchType: MatchEquals,
						}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-email-equals-not",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:      "example.com",
							MatchType: MatchEquals,
						}},
					},
				},
			},
			addr: alice,
			want: false,
		},
		{
			name: "match-email-equals-ok-negate",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:            "bob@example.com",
							NegateCondition: true,
							MatchType:       MatchEquals,
						}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-email-starts-with-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:      "alice@",
							MatchType: MatchStartsWith,
						}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-email-ends-with-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:      "com",
							MatchType: MatchEndsWith,
						}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-email-ends-with-not",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldEmail,
						TextMatches: []TextMatch{{
							Text:      ".org",
							MatchType: MatchEndsWith,
						}},
					},
				},
			},
			addr: alice,
			want: false,
		},
		{
			name: "match-name-contains-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldName,
						TextMatches: []TextMatch{{
							Text:      "Alice",
							MatchType: MatchContains,
						}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-name-contains-all-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldName,
						Test: FilterAllOf,
						TextMatches: []TextMatch{
							{
								Text:      "Alice",
								MatchType: MatchContains,
							},
							{
								Text:      "Gopher",
								MatchType: MatchContains,
							},
						},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-name-contains-all-prop-not",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				FilterTest: FilterAllOf,
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldName,
						TextMatches: []TextMatch{{
							Text:      "Alice",
							MatchType: MatchContains,
						}},
					},
					{
						Name: vcard.FieldName,
						TextMatches: []TextMatch{{
							Text:      "GopherXXX",
							MatchType: MatchContains,
						}},
					},
				},
			},
			addr: alice,
			want: false,
		},
		{
			name: "match-name-contains-all-text-match-not",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				PropFilters: []PropFilter{
					{
						Name: vcard.FieldName,
						Test: FilterAllOf,
						TextMatches: []TextMatch{
							{
								Text:      "Alice",
								MatchType: MatchContains,
							},
							{
								Text:      "GopherXXX",
								MatchType: MatchContains,
							},
						},
					},
				},
			},
			addr: alice,
			want: false,
		},
		{
			name: "missing-prop-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
						"XXX-not-THERE", // but AllProp is false.
					},
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "match-all-prop-ok",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
					AllProp: true,
				},
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addr: alice,
			want: true,
		},
		{
			name: "invalid-query-filter",
			query: &AddressBookQuery{
				DataRequest: AddressDataRequest{
					Props: []string{
						vcard.FieldFormattedName,
						vcard.FieldEmail,
						vcard.FieldUID,
					},
				},
				FilterTest: "XXX-invalid-filter",
				PropFilters: []PropFilter{
					{
						Name:        vcard.FieldEmail,
						TextMatches: []TextMatch{{Text: "example.com"}},
					},
				},
			},
			addr: alice,
			err:  fmt.Errorf("unknown query filter test \"XXX-invalid-filter\""),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Match(tc.query, &tc.addr)
			switch {
			case err != nil && tc.err == nil:
				t.Fatalf("unexpected error: %+v", err)
			case err != nil && tc.err != nil:
				if got, want := err.Error(), tc.err.Error(); got != want {
					t.Fatalf("invalid error:\ngot= %q\nwant=%q", got, want)
				}
			case err == nil && tc.err != nil:
				t.Fatalf("expected an error:\ngot= %+v\nwant=%+v", err, tc.err)
			case err == nil && tc.err == nil:
				if got, want := got, tc.want; got != want {
					t.Fatalf("invalid match value: got=%v, want=%v", got, want)
				}
			}
		})
	}
}
