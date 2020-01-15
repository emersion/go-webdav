package webdav

import (
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"

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
	s := fmt.Sprintf("%v %v", err.Code, http.StatusText(err.Code))
	if err.Err != nil {
		return fmt.Sprintf("%v: %v", s, err.Err)
	} else {
		return s
	}
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

	// TODO: refuse DepthInfinity, can cause infinite loops with symlinks

	f, err := h.FileSystem.Open(r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	var resps []internal.Response
	if err := h.propfind(&propfind, r.URL.Path, fi, depth, &resps); err != nil {
		return err
	}

	ms := internal.NewMultistatus(resps...)

	w.Header().Add("Content-Type", "text/xml; charset=\"utf-8\"")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(xml.Header))
	return xml.NewEncoder(w).Encode(&ms)
}

func (h *Handler) propfind(propfind *internal.Propfind, name string, fi os.FileInfo, depth internal.Depth, resps *[]internal.Response) error {
	// TODO: use partial error Response on error

	resp, err := h.propfindFile(propfind, name, fi)
	if err != nil {
		return err
	}
	*resps = append(*resps, *resp)

	if depth != internal.DepthZero && fi.IsDir() {
		childDepth := depth
		if depth == internal.DepthOne {
			childDepth = internal.DepthZero
		}

		f, err := h.FileSystem.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()

		children, err := f.Readdir(-1)
		if err != nil {
			return err
		}

		for _, child := range children {
			if err := h.propfind(propfind, path.Join(name, child.Name()), child, childDepth, resps); err != nil {
				return err
			}
		}
	}

	return nil
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
					// TODO: don't throw away error message here
					if httpErr, ok := err.(*HTTPError); ok {
						code = httpErr.Code
					} else {
						code = http.StatusInternalServerError
					}
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
	{"DAV:", "getcontentlength"}: func(h *Handler, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetContentLength{Length: fi.Size()}, nil
	},
	{"DAV:", "getcontenttype"}: func(h *Handler, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &HTTPError{Code: http.StatusNotFound}
		}
		t := mime.TypeByExtension(path.Ext(name))
		if t == "" {
			// TODO: use http.DetectContentType
			return nil, &HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetContentType{Type: t}, nil
	},
	{"DAV:", "getlastmodified"}: func(h *Handler, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetLastModified{LastModified: internal.Time(fi.ModTime())}, nil
	},
	// TODO: getetag
}
