package webdav

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockDeadPropFS struct {
	LocalFileSystem
	props map[string]map[xml.Name]interface{}
}

func (m *mockDeadPropFS) GetDeadProperties(ctx context.Context, name string) (map[xml.Name]interface{}, error) {
	if p, ok := m.props[name]; ok {
		return p, nil
	}
	return nil, nil
}

func TestPropFindDeadProperties(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &mockDeadPropFS{
		LocalFileSystem: LocalFileSystem(tmpDir),
		props: map[string]map[xml.Name]interface{}{
			"/": {
				{Space: "urn:schemas-microsoft-com:", Local: "ishidden"}: "1",
			},
		},
	}

	handler := Handler{
		FileSystem: fs,
	}

	propFindXML := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:" xmlns:ns="urn:schemas-microsoft-com:">
	 <D:prop>
	   <ns:ishidden/>
	 </D:prop>
</D:propfind>`

	req := httptest.NewRequest("PROPFIND", "/", strings.NewReader(propFindXML))
	req.Header.Set("Content-Type", "application/xml")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusMultiStatus {
		t.Fatalf("expected status MultiStatus (207), got %v", res.StatusCode)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "ishidden") || !strings.Contains(body, "1") {
		t.Errorf("expected dead property 'ishidden' with value '1' in response, got %s", body)
	}
}
