package webdav

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerCopyMoveOntoItself(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(&Handler{FileSystem: LocalFileSystem(dir)})
	defer ts.Close()

	for _, method := range []string{"COPY", "MOVE"} {
		req, err := http.NewRequest(method, ts.URL+"/a.txt", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Destination", ts.URL+"/a.txt")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%v onto itself: status = %v, want %v", method, resp.StatusCode, http.StatusForbidden)
		}
	}

	if got, err := os.ReadFile(p); err != nil || string(got) != "a" {
		t.Errorf("a.txt = %q, %v; want %q", got, err, "a")
	}
}
