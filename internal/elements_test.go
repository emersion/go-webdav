package internal

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"
)

// https://tools.ietf.org/html/rfc4918#section-9.6.2
const exampleDeleteMultistatusStr = `<?xml version="1.0" encoding="utf-8" ?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>http://www.example.com/container/resource3</d:href>
    <d:status>HTTP/1.1 423 Locked</d:status>
    <d:error><d:lock-token-submitted/></d:error>
  </d:response>
</d:multistatus>`

func TestResponse_Err_error(t *testing.T) {
	r := strings.NewReader(exampleDeleteMultistatusStr)
	var ms MultiStatus
	if err := xml.NewDecoder(r).Decode(&ms); err != nil {
		t.Fatalf("Decode() = %v", err)
	}

	if len(ms.Responses) != 1 {
		t.Fatalf("expected 1 <response>, got %v", len(ms.Responses))
	}

	resp := ms.Responses[0]

	err := resp.Err()
	var httpErr *HTTPError
	if err == nil {
		t.Errorf("Multistatus.Get() returned a nil error, expected non-nil")
	} else if !errors.As(err, &httpErr) {
		t.Errorf("Multistatus.Get() = %T, expected an *HTTPError", err)
	} else if httpErr.Code != 423 {
		t.Errorf("HTTPError.Code = %v, expected 423", httpErr.Code)
	}
}

func TestETagUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ETag
	}{
		{
			name:  "quoted (RFC-compliant)",
			input: `"abc123"`,
			want:  "abc123",
		},
		{
			name:  "unquoted (non-compliant server, e.g. mailbox.org)",
			input: `abc123`,
			want:  "abc123",
		},
		{
			name:  "quoted with special characters",
			input: `"etag/with-dashes_and.dots"`,
			want:  "etag/with-dashes_and.dots",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got ETag
			if err := got.UnmarshalText([]byte(tc.input)); err != nil {
				t.Fatalf("UnmarshalText(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("UnmarshalText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestETagRoundTrip(t *testing.T) {
	var etag ETag
	if err := etag.UnmarshalText([]byte(`"abc123"`)); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	b, err := etag.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	if string(b) != `"abc123"` {
		t.Errorf("MarshalText = %q, want %q", b, `"abc123"`)
	}
}

func TestTimeRoundTrip(t *testing.T) {
	now := Time(time.Now().UTC())
	want, err := now.MarshalText()
	if err != nil {
		t.Fatalf("could not marshal time: %+v", err)
	}

	var got Time
	err = got.UnmarshalText(want)
	if err != nil {
		t.Fatalf("could not unmarshal time: %+v", err)
	}

	raw, err := got.MarshalText()
	if err != nil {
		t.Fatalf("could not marshal back: %+v", err)
	}

	if got, want := raw, want; !bytes.Equal(got, want) {
		t.Fatalf("invalid round-trip:\ngot= %s\nwant=%s", got, want)
	}
}
