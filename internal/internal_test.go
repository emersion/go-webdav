package internal

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseConditions(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		conditions [][]Condition
	}{
		{
			name: "RFC 4918 section 10.4.6: No-tag Production",
			s: `(<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>
  ["I am an ETag"])
  (["I am another ETag"])`,
			conditions: [][]Condition{
				{
					{Token: "urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"},
					{ETag: `"I am an ETag"`},
				},
				{
					{ETag: `"I am another ETag"`},
				},
			},
		},
		{
			name: `RFC 4918 section 10.4.7: Using "Not" with No-tag Production`,
			s: `(Not <urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>
  <urn:uuid:58f202ac-22cf-11d1-b12d-002035b29092>)`,
			conditions: [][]Condition{
				{
					{Not: true, Token: "urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"},
					{Token: "urn:uuid:58f202ac-22cf-11d1-b12d-002035b29092"},
				},
			},
		},
		{
			name: "RFC 4918 section 10.4.8: Causing a Condition to Always Evaluate to True",
			s: `(<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>)
  (Not <DAV:no-lock>)`,
			conditions: [][]Condition{
				{
					{Token: "urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"},
				},
				{
					{Not: true, Token: "DAV:no-lock"},
				},
			},
		},
		{
			name: "RFC 4918 section 10.4.9: Tagged List If Header in COPY",
			s: `</resource1>
  (<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>
  [W/"A weak ETag"]) (["strong ETag"])`,
			conditions: [][]Condition{
				{
					{Resource: "/resource1", Token: "urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"},
					{Resource: "/resource1", ETag: `W/"A weak ETag"`},
				},
				{
					{ETag: `"strong ETag"`},
				},
			},
		},
		{
			name: "RFC 4918 section 10.4.10: Matching Lock Tokens with Collection Locks",
			s: `<http://www.example.com/specs/>
  (<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>)`,
			conditions: [][]Condition{
				{
					{Resource: "http://www.example.com/specs/", Token: "urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"},
				},
			},
		},
		{
			name: "RFC 4918 section 10.4.11: Matching ETags on Unmapped URLs",
			s:    `</specs/rfc2518.doc> (["4217"])`,
			conditions: [][]Condition{
				{
					{Resource: "/specs/rfc2518.doc", ETag: `"4217"`},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := ParseConditions(strings.ReplaceAll(tc.s, "\n", " "))
			if err != nil {
				t.Fatalf("ParseConditions() = %v", err)
			} else if !reflect.DeepEqual(l, tc.conditions) {
				t.Errorf("ParseConditions() = \n %#v \n but want: \n %#v", l, tc.conditions)
			}
		})
	}
}
