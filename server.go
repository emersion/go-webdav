package webdav

import (
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"os"

	"github.com/emersion/go-webdav/internal"
)

type HTTPError struct {
	Code int
	Err  error
}

func HTTPErrorf(code int, format string, a ...interface{}) *HTTPError {
	return &HTTPError{code, fmt.Errorf(format, a...)}
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("%v %v: %v", err.Code, http.StatusText(err.Code), err.Err)
}

type File interface {
	http.File
}

type FileSystem interface {
	Open(name string) (File, error)
}

type Handler struct {
	FileSystem FileSystem
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	if h.FileSystem == nil {
		err = HTTPErrorf(http.StatusInternalServerError, "webdav: no filesystem available")
	} else {
		switch r.Method {
		case http.MethodOptions:
			err = h.handleOptions(w, r)
		case http.MethodGet, http.MethodHead:
			err = h.handleGetHead(w, r)
		case "PROPFIND":
			err = h.handlePropfind(w, r)
		default:
			err = HTTPErrorf(http.StatusMethodNotAllowed, "webdav: unsupported method")
		}
	}

	if err != nil {
		code := http.StatusInternalServerError
		if httpErr, ok := err.(*HTTPError); ok {
			code = httpErr.Code
		}
		http.Error(w, err.Error(), code)
	}
}

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) error {
	w.Header().Add("Allow", "OPTIONS, GET, HEAD")
	w.Header().Add("DAV", "1")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) handleGetHead(w http.ResponseWriter, r *http.Request) error {
	f, err := h.FileSystem.Open(r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	http.ServeContent(w, r, r.URL.Path, fi.ModTime(), f)
	return nil
}

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request) error {
	t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if t != "application/xml" && t != "text/xml" {
		return HTTPErrorf(http.StatusBadRequest, "webdav: expected application/xml PROPFIND request")
	}

	var propfind internal.Propfind
	if err := xml.NewDecoder(r.Body).Decode(&propfind); err != nil {
		return &HTTPError{http.StatusBadRequest, err}
	}

	depth := internal.DepthInfinity
	if s := r.Header.Get("Depth"); s != "" {
		var err error
		depth, err = internal.ParseDepth(s)
		if err != nil {
			return &HTTPError{http.StatusBadRequest, err}
		}
	}

	f, err := h.FileSystem.Open(r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	if depth != internal.DepthZero {
		depth = internal.DepthZero // TODO
	}

	resp, err := h.propfindFile(&propfind, r.URL.Path, fi)
	if err != nil {
		return err
	}

	ms := internal.NewMultistatus(*resp)

	w.Header().Add("Content-Type", "text/xml; charset=\"utf-8\"")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(xml.Header))
	return xml.NewEncoder(w).Encode(&ms)
}

func (h *Handler) propfindFile(propfind *internal.Propfind, name string, fi os.FileInfo) (*internal.Response, error) {
	resp := internal.NewOKResponse(name)

	if prop := propfind.Prop; prop != nil {
		for _, xmlName := range prop.XMLNames() {
			emptyVal := internal.NewRawXMLElement(xmlName, nil, nil)

			var code int
			var val interface{} = emptyVal
			f, ok := liveProps[xmlName]
			if ok {
				if v, err := f(h, name, fi); err != nil {
					code = http.StatusInternalServerError // TODO: better error handling
				} else {
					code = http.StatusOK
					val = v
				}
			} else {
				code = http.StatusNotFound
			}

			if err := resp.EncodeProp(code, val); err != nil {
				return nil, err
			}
		}
	}

	return resp, nil
}

type PropfindFunc func(h *Handler, name string, fi os.FileInfo) (interface{}, error)

var liveProps = map[xml.Name]PropfindFunc{
	{"DAV:", "resourcetype"}: func(h *Handler, name string, fi os.FileInfo) (interface{}, error) {
		var types []xml.Name
		if fi.IsDir() {
			types = append(types, internal.CollectionName)
		}
		return internal.NewResourceType(types...), nil
	},
}
