package carddav

import (
	"encoding/xml"
	"net/http"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple address books

type Backend interface {
	GetAddressObject(href string) (*AddressObject, error)
	ListAddressObjects() ([]AddressObject, error)
}

type Handler struct {
	Backend Backend
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Backend == nil {
		http.Error(w, "carddav: no backend available", http.StatusInternalServerError)
		return
	}

	b := backend{h.Backend}
	hh := internal.Handler{&b}
	hh.ServeHTTP(w, r)
}

type backend struct {
	Backend Backend
}

func (b *backend) Options(r *http.Request) ([]string, error) {
	// TODO: add DAV: addressbook

	if r.URL.Path == "/" {
		return []string{http.MethodOptions, "PROPFIND"}, nil
	}

	_, err := b.Backend.GetAddressObject(r.URL.Path)
	if httpErr, ok := err.(*internal.HTTPError); ok && httpErr.Code == http.StatusNotFound {
		return []string{http.MethodOptions}, nil
	} else if err != nil {
		return nil, err
	}

	return []string{http.MethodOptions, http.MethodHead, http.MethodGet, "PROPFIND"}, nil
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		return &internal.HTTPError{Code: http.StatusMethodNotAllowed}
	}

	ao, err := b.Backend.GetAddressObject(r.URL.Path)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", vcard.MIMEType)
	// TODO: set ETag, Last-Modified

	if r.Method != http.MethodHead {
		return vcard.NewEncoder(w).Encode(ao.Card)
	}
	return nil
}

func (b *backend) Propfind(r *http.Request, propfind *internal.Propfind, depth internal.Depth) (*internal.Multistatus, error) {
	var resps []internal.Response
	if r.URL.Path == "/" {
		resp, err := b.propfindAddressBook(propfind)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)

		if depth != internal.DepthZero {
			aos, err := b.Backend.ListAddressObjects()
			if err != nil {
				return nil, err
			}

			for _, ao := range aos {
				resp, err := b.propfindAddressObject(propfind, &ao)
				if err != nil {
					return nil, err
				}
				resps = append(resps, *resp)
			}
		}
	} else {
		ao, err := b.Backend.GetAddressObject(r.URL.Path)
		if err != nil {
			return nil, err
		}

		resp, err := b.propfindAddressObject(propfind, ao)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfindAddressBook(propfind *internal.Propfind) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
		internal.ResourceTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return internal.NewResourceType(internal.CollectionName, addressBookName), nil
		},
	}

	return internal.NewPropfindResponse("/", propfind, props)
}

func (b *backend) propfindAddressObject(propfind *internal.Propfind, ao *AddressObject) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
		internal.GetContentTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetContentType{Type: vcard.MIMEType}, nil
		},
		// TODO getlastmodified, getetag
	}

	return internal.NewPropfindResponse(ao.Href, propfind, props)
}
