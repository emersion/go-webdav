package webdav

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var lock = `
<?xml version="1.0" encoding="UTF-8"?>
<A:lockinfo xmlns:A='DAV:'>
  <A:lockscope><A:exclusive/></A:lockscope>
  <A:locktype><A:write/></A:locktype>
</A:lockinfo>
`

func TestLock(t *testing.T) {
	req := httptest.NewRequest("LOCK", "/res", strings.NewReader(lock))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler := &Handler{FileSystem: testFileSystem{}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	resp := string(data)
	if res.StatusCode != 200 {
		t.Errorf("Bad status returned when doing a LOCK:\n%d", res.StatusCode)
	}
	if !strings.Contains(resp, `<lockroot xmlns="DAV:"><href>/res</href></lockroot>`) {
		t.Errorf("Bad lockroot returned when doing a LOCK, response:\n%s", resp)
	}
	if !strings.Contains(resp, `<depth>infinity</depth>`) {
		t.Errorf("Bad depth returned when doing a LOCK, response:\n%s", resp)
	}
	tok := res.Header.Get("Lock-Token")
	if len(tok) < 2 {
		t.Error("No token in Lock-Token header when doing a LOCK")
	} else if !strings.Contains(resp, tok[1:len(tok)-1]) {
		t.Errorf("Token not in body when doing a LOCK, response:\n%s", resp)
	}
}

func TestLockConflict(t *testing.T) {
	handler := &Handler{FileSystem: testFileSystem{}}

	req := httptest.NewRequest("LOCK", "/res", strings.NewReader(lock))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	req = httptest.NewRequest("LOCK", "/res", strings.NewReader(lock))
	req.Header.Set("Content-Type", "application/xml")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusLocked {
		t.Errorf("Bad status returned when creating a conflicting lock:\n%d", res.StatusCode)
	}
}

func TestLockRefreshBadToken(t *testing.T) {
	req := httptest.NewRequest("LOCK", "/res", strings.NewReader(""))
	req.Header.Set("If", "(<opaquelocktoken:anytoken>)")
	w := httptest.NewRecorder()
	handler := &Handler{FileSystem: testFileSystem{}}
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("Bad status returned when refreshing a lock with bad token:\n%d", res.StatusCode)
	}
}

type testFileSystem struct{}

func (fs testFileSystem) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	return nil, nil
}

func (fs testFileSystem) Stat(ctx context.Context, name string) (*FileInfo, error) {
	fi := &FileInfo{Path: name}
	return fi, nil
}

func (fs testFileSystem) ReadDir(ctx context.Context, name string, recursive bool) ([]FileInfo, error) {
	return nil, nil
}

func (fs testFileSystem) Create(ctx context.Context, name string, body io.ReadCloser, opts *CreateOptions) (fileInfo *FileInfo, created bool, err error) {
	return nil, false, nil
}

func (fs testFileSystem) RemoveAll(ctx context.Context, name string, opts *RemoveAllOptions) error {
	return nil
}

func (fs testFileSystem) Mkdir(ctx context.Context, name string) error {
	return nil
}

func (fs testFileSystem) Copy(ctx context.Context, name, dest string, options *CopyOptions) (created bool, err error) {
	return false, nil
}

func (fs testFileSystem) Move(ctx context.Context, name, dest string, options *MoveOptions) (created bool, err error) {
	return false, nil
}
