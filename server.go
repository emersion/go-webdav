package webdav

import (
	"encoding/xml"
	"mime"
	"net/http"
	"os"
	"path"

	"github.com/emersion/go-webdav/internal"
)

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

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	f, err := b.FileSystem.Open(r.URL.Path)
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

func (b *backend) Propfind(r *http.Request, propfind *internal.Propfind, depth internal.Depth) (*internal.Multistatus, error) {
	f, err := b.FileSystem.Open(r.URL.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
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
	resp := internal.NewOKResponse(name)

	if propfind.PropName != nil {
		for xmlName, f := range liveProps {
			emptyVal := internal.NewRawXMLElement(xmlName, nil, nil)

			_, err := f(b, name, fi)
			if err != nil {
				continue
			}

			if err := resp.EncodeProp(http.StatusOK, emptyVal); err != nil {
				return nil, err
			}
		}
	} else if propfind.AllProp != nil {
		// TODO: add support for propfind.Include
		for _, f := range liveProps {
			val, err := f(b, name, fi)
			if err != nil {
				continue
			}

			if err := resp.EncodeProp(http.StatusOK, val); err != nil {
				return nil, err
			}
		}
	} else if prop := propfind.Prop; prop != nil {
		for _, xmlName := range prop.XMLNames() {
			emptyVal := internal.NewRawXMLElement(xmlName, nil, nil)

			var code int
			var val interface{} = emptyVal
			f, ok := liveProps[xmlName]
			if ok {
				if v, err := f(b, name, fi); err != nil {
					// TODO: don't throw away error message here
					if httpErr, ok := err.(*internal.HTTPError); ok {
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
	} else {
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "webdav: propfind request missing propname, allprop or prop element")
	}

	return resp, nil
}

type PropfindFunc func(b *backend, name string, fi os.FileInfo) (interface{}, error)

var liveProps = map[xml.Name]PropfindFunc{
	{"DAV:", "resourcetype"}: func(b *backend, name string, fi os.FileInfo) (interface{}, error) {
		var types []xml.Name
		if fi.IsDir() {
			types = append(types, internal.CollectionName)
		}
		return internal.NewResourceType(types...), nil
	},
	{"DAV:", "getcontentlength"}: func(b *backend, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &internal.HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetContentLength{Length: fi.Size()}, nil
	},
	{"DAV:", "getcontenttype"}: func(b *backend, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &internal.HTTPError{Code: http.StatusNotFound}
		}
		t := mime.TypeByExtension(path.Ext(name))
		if t == "" {
			// TODO: use http.DetectContentType
			return nil, &internal.HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetContentType{Type: t}, nil
	},
	{"DAV:", "getlastmodified"}: func(b *backend, name string, fi os.FileInfo) (interface{}, error) {
		if fi.IsDir() {
			return nil, &internal.HTTPError{Code: http.StatusNotFound}
		}
		return &internal.GetLastModified{LastModified: internal.Time(fi.ModTime())}, nil
	},
	// TODO: getetag
}
