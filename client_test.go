package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRemoveAll(t *testing.T) {
	for _, tc := range []struct {
		name            string
		opts            *RemoveAllOptions
		wantIfMatch     string
		wantIfNoneMatch string
	}{
		{name: "nil options"},
		{
			name:        "If-Match",
			opts:        &RemoveAllOptions{IfMatch: `"abc"`},
			wantIfMatch: `"abc"`,
		},
		{
			name:            "If-None-Match",
			opts:            &RemoveAllOptions{IfNoneMatch: "*"},
			wantIfNoneMatch: "*",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotIfMatch, gotIfNoneMatch string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotIfMatch = r.Header.Get("If-Match")
				gotIfNoneMatch = r.Header.Get("If-None-Match")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			c, err := NewClient(nil, ts.URL)
			if err != nil {
				t.Fatalf("NewClient() = %v", err)
			}
			if err := c.RemoveAll(context.Background(), "/file", tc.opts); err != nil {
				t.Fatalf("RemoveAll() = %v", err)
			}

			if gotMethod != http.MethodDelete {
				t.Errorf("method = %q, want %q", gotMethod, http.MethodDelete)
			}
			if gotIfMatch != tc.wantIfMatch {
				t.Errorf("If-Match = %q, want %q", gotIfMatch, tc.wantIfMatch)
			}
			if gotIfNoneMatch != tc.wantIfNoneMatch {
				t.Errorf("If-None-Match = %q, want %q", gotIfNoneMatch, tc.wantIfNoneMatch)
			}
		})
	}
}

func TestClientRemoveAllPreconditionFailed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer ts.Close()

	c, err := NewClient(nil, ts.URL)
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	err = c.RemoveAll(context.Background(), "/file", &RemoveAllOptions{IfMatch: `"abc"`})
	if code := HTTPErrorCode(err); code != http.StatusPreconditionFailed {
		t.Fatalf("RemoveAll() = %v, want HTTP error with status %v", err, http.StatusPreconditionFailed)
	}
}
