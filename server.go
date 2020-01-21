package webdav

import (
	"encoding/xml"
	"io"
	"mime"
	"net/http"
	"os"
	"path"

	"github.com/emersion/go-webdav/internal"
)

type File interface {
	io.Closer
	io.Reader
	io.Seeker
	Readdir(count int) ([]os.FileInfo, error)
}

type FileSystem interface {
	Open(name string) (File, error)
	Stat(name string) (os.FileInfo, error)
}

type Handler struct {
	FileSystem FileSystem
}

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

	if fi.IsDir() {
		return []string{http.MethodOptions, "PROPFIND"}, nil
	} else {
		return []string{http.MethodOptions, http.MethodHead, http.MethodGet, "PROPFIND"}, nil
	}
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	fi, err := b.FileSystem.Stat(r.URL.Path)
	if os.IsNotExist(err) {
		return &internal.HTTPError{Code: http.StatusNotFound, Err: err}
	} else if err != nil {
		return err
	}
	if fi.IsDir() {
		return &internal.HTTPError{Code: http.StatusMethodNotAllowed}
	}

	f, err := b.FileSystem.Open(r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	http.ServeContent(w, r, r.URL.Path, fi.ModTime(), f)
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
	if err := b.propfind(propfind, r.URL.Path, fi, depth, &resps); err != nil {
		return nil, err
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfind(propfind *internal.Propfind, name string, fi os.FileInfo, depth internal.Depth, resps *[]internal.Response) error {
	// TODO: use partial error Response on error

	resp, err := b.propfindFile(propfind, name, fi)
	if err != nil {
		return err
	}
	*resps = append(*resps, *resp)

	if depth != internal.DepthZero && fi.IsDir() {
		childDepth := depth
		if depth == internal.DepthOne {
			childDepth = internal.DepthZero
		}

		f, err := b.FileSystem.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()

		children, err := f.Readdir(-1)
		if err != nil {
			return err
		}

		for _, child := range children {
			if err := b.propfind(propfind, path.Join(name, child.Name()), child, childDepth, resps); err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *backend) propfindFile(propfind *internal.Propfind, name string, fi os.FileInfo) (*internal.Response, error) {
	props := make(map[xml.Name]internal.PropfindFunc)

	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		var types []xml.Name
		if fi.IsDir() {
			types = append(types, internal.CollectionName)
		}
		return internal.NewResourceType(types...), nil
	}

	if !fi.IsDir() {
		props[internal.GetContentLengthName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetContentLength{Length: fi.Size()}, nil
		}
		props[internal.GetContentTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
			t := mime.TypeByExtension(path.Ext(name))
			if t == "" {
				// TODO: use http.DetectContentType
				return nil, &internal.HTTPError{Code: http.StatusNotFound}
			}
			return &internal.GetContentType{Type: t}, nil
		}
		props[internal.GetLastModifiedName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetLastModified{LastModified: internal.Time(fi.ModTime())}, nil
		}
		// TODO: getetag
	}

	return internal.NewPropfindResponse(name, propfind, props)
}
