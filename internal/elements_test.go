package internal

import (
	"bytes"
	"encoding/xml"
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
	if err == nil {
		t.Errorf("Multistatus.Get() returned a nil error, expected non-nil")
	} else if httpErr, ok := err.(*HTTPError); !ok {
		t.Errorf("Multistatus.Get() = %T, expected an *HTTPError", err)
	} else if httpErr.Code != 423 {
		t.Errorf("HTTPError.Code = %v, expected 423", httpErr.Code)
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

func TestETag_UnmarshalText(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name     string
		etag     ETag
		args     args
		wantErr  bool
		wantETag ETag
	}{
		{
			name: "double quoted string",
			etag: "",
			args: args{
				b: []byte("\"1692394723948\""),
			},
			wantErr:  false,
			wantETag: "1692394723948",
		},
		{
			name: "empty double quoted string",
			etag: "",
			args: args{
				b: []byte("\"\""),
			},
			wantErr:  false,
			wantETag: "",
		},
		{
			name: "unquoted string",
			etag: "",
			args: args{
				b: []byte("1692394723948"),
			},
			wantErr:  false,
			wantETag: "1692394723948",
		},
		{
			name: "empty string",
			etag: "",
			args: args{
				b: []byte(""),
			},
			wantErr:  false,
			wantETag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.etag.UnmarshalText(tt.args.b); (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.etag != tt.wantETag {
				t.Errorf("UnmarshalText() want %s, got %s", tt.wantETag, tt.etag)
			}
		})
	}
}

func TestETag_String(t *testing.T) {
	tests := []struct {
		name string
		etag ETag
		want string
	}{
		{
			name: "string with double-quote",
			etag: "162392347123",
			want: "\"162392347123\"",
		},
		{
			name: "empty string with double-quote",
			etag: "",
			want: "\"\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.etag.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestETag_UnmarshalAndMarshalText(t *testing.T) {
	tests := []struct {
		name string
		// initial ETag
		etag ETag
		// first value for ETag UnmarshalText
		unmarshalValue []byte
		// expected MarshalText() result after UnmarshalText()
		wantMarshalled []byte
		// expected UnmarshalText() result after MarshalText() with unmarshalValue
		wantUnmarshalledETag ETag
	}{
		{
			name:                 "string",
			etag:                 "",
			unmarshalValue:       []byte("162392347123"),
			wantMarshalled:       []byte("\"162392347123\""),
			wantUnmarshalledETag: "162392347123",
		},
		{
			name:                 "double-quoted",
			etag:                 "",
			unmarshalValue:       []byte("\"162392347123\""),
			wantMarshalled:       []byte("\"162392347123\""),
			wantUnmarshalledETag: "162392347123",
		},
		{
			name:                 "empty string",
			etag:                 "",
			unmarshalValue:       []byte(""),
			wantMarshalled:       []byte("\"\""),
			wantUnmarshalledETag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.etag.UnmarshalText(tt.unmarshalValue); err != nil {
				t.Errorf("UnmarshalText() = %v", err)
			}

			m, err := tt.etag.MarshalText()
			if err != nil {
				t.Errorf("MarshalText() = %v", err)
			}

			if !bytes.Equal(m, tt.wantMarshalled) {
				t.Errorf("MarshalText() want %s, got %s", tt.wantMarshalled, m)
			}

			if err := tt.etag.UnmarshalText(m); err != nil {
				t.Errorf("UnmarshalText() got error after MarshalText() = %v", err)
			}

			if tt.etag != tt.wantUnmarshalledETag {
				t.Errorf("UnmarshalText() after MarshalText() want %s, got %s", tt.wantUnmarshalledETag, tt.etag)
			}
		})
	}
}
