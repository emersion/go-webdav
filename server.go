package webdav

import (
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/emersion/go-webdav/internal"
)

// FileSystem is a WebDAV server backend.
type FileSystem interface {
	Open(name string) (io.ReadCloser, error)
	Stat(name string) (*FileInfo, error)
	Readdir(name string, recursive bool) ([]FileInfo, error)
	Create(name string) (io.WriteCloser, error)
	RemoveAll(name string) error
	Mkdir(name string) error
	Copy(name, dest string, recursive, overwrite bool) (created bool, err error)
	MoveAll(name, dest string, overwrite bool) (created bool, err error)
}

// Handler handles WebDAV HTTP requests. It can be used to create a WebDAV
// server.
type Handler struct {
	FileSystem FileSystem
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.FileSystem == nil {
		http.Error(w, "webdav: no filesystem available", http.StatusInternalServerError)
		return
	}

	b := backend{h.FileSystem}
	hh := internal.Handler{&b}
	hh.ServeHTTP(w, r)
}

// NewHTTPError creates a new error that is associated with an HTTP status code
// and optionally an error that lead to it. Backends can use this functions to
// return errors that convey some semantics (e.g. 404 not found, 403 access
// denied, etc) while also providing an (optional) arbitrary error context
// (intended for humans).
func NewHTTPError(statusCode int, cause error) error {
	return &internal.HTTPError{Code: statusCode, Err: cause}
}

type backend struct {
	FileSystem FileSystem
}

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return nil, []string{http.MethodOptions, http.MethodPut, "MKCOL"}, nil
	} else if err != nil {
		return nil, nil, err
	}

	allow = []string{
		http.MethodOptions,
		http.MethodDelete,
		"PROPFIND",
		"COPY",
		"MOVE",
	}

	if !fi.IsDir {
		allow = append(allow, http.MethodHead, http.MethodGet, http.MethodPut)
	}

	return nil, allow, nil
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return &internal.HTTPError{Code: http.StatusNotFound, Err: err}
	} else if err != nil {
		return err
	}
	if fi.IsDir {
		return &internal.HTTPError{Code: http.StatusMethodNotAllowed}
	}

	f, err := b.FileSystem.Open(r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))
	if fi.MIMEType != "" {
		w.Header().Set("Content-Type", fi.MIMEType)
	}
	if !fi.ModTime.IsZero() {
		w.Header().Set("Last-Modified", fi.ModTime.UTC().Format(http.TimeFormat))
	}
	if fi.ETag != "" {
		w.Header().Set("ETag", internal.ETag(fi.ETag).String())
	}

	if rs, ok := f.(io.ReadSeeker); ok {
		// If it's an io.Seeker, use http.ServeContent which supports ranges
		http.ServeContent(w, r, r.URL.Path, fi.ModTime, rs)
	} else {
		if r.Method != http.MethodHead {
			io.Copy(w, f)
		}
	}
	return nil
}

func (b *backend) Propfind(r *http.Request, propfind *internal.Propfind, depth internal.Depth) (*internal.Multistatus, error) {
	// TODO: use partial error Response on error

	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return nil, &internal.HTTPError{Code: http.StatusNotFound, Err: err}
	} else if err != nil {
		return nil, err
	}

	var resps []internal.Response
	if depth != internal.DepthZero && fi.IsDir {
		children, err := b.FileSystem.Readdir(r.URL.Path, depth == internal.DepthInfinity)
		if err != nil {
			return nil, err
		}

		resps = make([]internal.Response, len(children))
		for i, child := range children {
			resp, err := b.propfindFile(propfind, &child)
			if err != nil {
				return nil, err
			}
			resps[i] = *resp
		}
	} else {
		resp, err := b.propfindFile(propfind, fi)
		if err != nil {
			return nil, err
		}

		resps = []internal.Response{*resp}
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfindFile(propfind *internal.Propfind, fi *FileInfo) (*internal.Response, error) {
	props := make(map[xml.Name]internal.PropfindFunc)

	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		var types []xml.Name
		if fi.IsDir {
			types = append(types, internal.CollectionName)
		}
		return internal.NewResourceType(types...), nil
	}

	if !fi.IsDir {
		props[internal.GetContentLengthName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetContentLength{Length: fi.Size}, nil
		}

		if !fi.ModTime.IsZero() {
			props[internal.GetLastModifiedName] = func(*internal.RawXMLValue) (interface{}, error) {
				return &internal.GetLastModified{LastModified: internal.Time(fi.ModTime)}, nil
			}
		}

		if fi.MIMEType != "" {
			props[internal.GetContentTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
				return &internal.GetContentType{Type: fi.MIMEType}, nil
			}
		}

		if fi.ETag != "" {
			props[internal.GetETagName] = func(*internal.RawXMLValue) (interface{}, error) {
				return &internal.GetETag{ETag: internal.ETag(fi.ETag)}, nil
			}
		}
	}

	return internal.NewPropfindResponse(fi.Path, propfind, props)
}

func (b *backend) Proppatch(r *http.Request, update *internal.Propertyupdate) (*internal.Response, error) {
	// TODO: return a failed Response instead
	return nil, internal.HTTPErrorf(http.StatusForbidden, "webdav: PROPPATCH is unsupported")
}

func (b *backend) Put(r *http.Request) (*internal.Href, error) {
	wc, err := b.FileSystem.Create(r.URL.Path)
	if err != nil {
		return nil, err
	}
	defer wc.Close()

	if _, err := io.Copy(wc, r.Body); err != nil {
		return nil, err
	}

	return nil, wc.Close()
}

func (b *backend) Delete(r *http.Request) error {
	err := b.FileSystem.RemoveAll(r.URL.Path)
	if os.IsNotExist(err) {
		return &internal.HTTPError{Code: http.StatusNotFound, Err: err}
	}
	return err
}

func (b *backend) Mkcol(r *http.Request) error {
	if r.Header.Get("Content-Type") != "" {
		return internal.HTTPErrorf(http.StatusUnsupportedMediaType, "webdav: request body not supported in MKCOL request")
	}
	err := b.FileSystem.Mkdir(r.URL.Path)
	if os.IsNotExist(err) {
		return &internal.HTTPError{Code: http.StatusConflict, Err: err}
	}
	return err
}

func (b *backend) Copy(r *http.Request, dest *internal.Href, recursive, overwrite bool) (created bool, err error) {
	created, err = b.FileSystem.Copy(r.URL.Path, dest.Path, recursive, overwrite)
	if os.IsExist(err) {
		return false, &internal.HTTPError{http.StatusPreconditionFailed, err}
	}
	return created, err
}

func (b *backend) Move(r *http.Request, dest *internal.Href, overwrite bool) (created bool, err error) {
	created, err = b.FileSystem.MoveAll(r.URL.Path, dest.Path, overwrite)
	if os.IsExist(err) {
		return false, &internal.HTTPError{http.StatusPreconditionFailed, err}
	}
	return created, err
}
