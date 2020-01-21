package webdav

import (
	"encoding/xml"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/emersion/go-webdav/internal"
)

// FileSystem is a WebDAV server backend.
type FileSystem interface {
	Open(name string) (io.ReadCloser, error)
	Stat(name string) (*FileInfo, error)
	Readdir(name string) ([]FileInfo, error)
	Create(name string) (io.WriteCloser, error)
	RemoveAll(name string) error
	Mkdir(name string) error
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

type backend struct {
	FileSystem FileSystem
}

func (b *backend) Options(r *http.Request) ([]string, error) {
	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return []string{http.MethodOptions}, nil
	} else if err != nil {
		return nil, err
	}

	if fi.IsDir {
		return []string{
			http.MethodOptions,
			http.MethodDelete,
			"PROPFIND",
			"MKCOL",
		}, nil
	} else {
		return []string{
			http.MethodOptions,
			http.MethodHead,
			http.MethodGet,
			http.MethodPut,
			http.MethodDelete,
			"PROPFIND",
		}, nil
	}
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

	if rs, ok := f.(io.ReadSeeker); ok {
		// If it's an io.Seeker, use http.ServeContent which supports ranges
		http.ServeContent(w, r, r.URL.Path, fi.ModTime, rs)
	} else {
		// TODO: fallback to http.DetectContentType
		t := mime.TypeByExtension(path.Ext(r.URL.Path))
		if t != "" {
			w.Header().Set("Content-Type", t)
		}

		if !fi.ModTime.IsZero() {
			w.Header().Set("Last-Modified", fi.ModTime.UTC().Format(http.TimeFormat))
		}

		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))

		if r.Method != http.MethodHead {
			io.Copy(w, f)
		}
	}
	return nil
}

func (b *backend) Propfind(r *http.Request, propfind *internal.Propfind, depth internal.Depth) (*internal.Multistatus, error) {
	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return nil, &internal.HTTPError{Code: http.StatusNotFound, Err: err}
	} else if err != nil {
		return nil, err
	}

	var resps []internal.Response
	if err := b.propfind(propfind, fi, depth, &resps); err != nil {
		return nil, err
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfind(propfind *internal.Propfind, fi *FileInfo, depth internal.Depth, resps *[]internal.Response) error {
	// TODO: use partial error Response on error

	resp, err := b.propfindFile(propfind, fi)
	if err != nil {
		return err
	}
	*resps = append(*resps, *resp)

	if depth != internal.DepthZero && fi.IsDir {
		childDepth := depth
		if depth == internal.DepthOne {
			childDepth = internal.DepthZero
		}

		children, err := b.FileSystem.Readdir(fi.Href)
		if err != nil {
			return err
		}

		for _, child := range children {
			if err := b.propfind(propfind, &child, childDepth, resps); err != nil {
				return err
			}
		}
	}

	return nil
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
		props[internal.GetContentTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
			t := mime.TypeByExtension(path.Ext(fi.Href))
			if t == "" {
				// TODO: use http.DetectContentType
				return nil, &internal.HTTPError{Code: http.StatusNotFound}
			}
			return &internal.GetContentType{Type: t}, nil
		}
		props[internal.GetLastModifiedName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetLastModified{LastModified: internal.Time(fi.ModTime)}, nil
		}
		// TODO: getetag
	}

	return internal.NewPropfindResponse(fi.Href, propfind, props)
}

func (b *backend) Put(r *http.Request) error {
	wc, err := b.FileSystem.Create(r.URL.Path)
	if err != nil {
		return err
	}
	defer wc.Close()

	if _, err := io.Copy(wc, r.Body); err != nil {
		return err
	}

	return wc.Close()
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
