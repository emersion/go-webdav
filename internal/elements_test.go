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

func TestMultistatus_Get_error(t *testing.T) {
	r := strings.NewReader(exampleDeleteMultistatusStr)
	var ms Multistatus
	if err := xml.NewDecoder(r).Decode(&ms); err != nil {
		t.Fatalf("Decode() = %v", err)
	}

	_, err := ms.Get("/container/resource3")
	if err == nil {
		t.Errorf("Multistatus.Get() returned a nil error, expected non-nil")
	} else if httpErr, ok := err.(*HTTPError); !ok {
		t.Errorf("Multistatus.Get() = %T, expected an *HTTPError", err)
	} else if httpErr.Code != 423 {
		t.Errorf("HTTPError.Code = %v, expected 423", httpErr.Code)
	}
}

func TestTimeRoundTrip(t *testing.T) {
	buf := new(bytes.Buffer)
	enc := xml.NewEncoder(buf)
	now := Time(time.Now())
	err := enc.Encode(now)
	if err != nil {
		t.Fatalf("could not encode time: %+v", err)
	}

	var got Time
	err = xml.NewDecoder(buf).Decode(&got)
	if err != nil {
		t.Fatalf("could not decode time: %+v", err)
	}

	if got, want := got, now; !got.Equal(want) {
		t.Fatalf("invalid round-trip:\ngot= %+v\nwant=%+v", got, want)
	}
}
