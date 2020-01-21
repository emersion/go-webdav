package carddav

import (
	"bytes"
	"encoding/xml"
	"net/http"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple address books

// Backend is a CardDAV server backend.
type Backend interface {
	AddressBook() (*AddressBook, error)
	GetAddressObject(href string) (*AddressObject, error)
	ListAddressObjects() ([]AddressObject, error)
	QueryAddressObjects(query *AddressBookQuery) ([]AddressObject, error)
}

// Handler handles CardDAV HTTP requests. It can be used to create a CardDAV
// server.
type Handler struct {
	Backend Backend
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Backend == nil {
		http.Error(w, "carddav: no backend available", http.StatusInternalServerError)
		return
	}

	var err error
	switch r.Method {
	case "REPORT":
		err = h.handleReport(w, r)
	default:
		b := backend{h.Backend}
		hh := internal.Handler{&b}
		hh.ServeHTTP(w, r)
	}

	if err != nil {
		internal.ServeError(w, err)
	}
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) error {
	var report reportReq
	if err := internal.DecodeXMLRequest(r, &report); err != nil {
		return err
	}

	if report.Query != nil {
		return h.handleQuery(w, report.Query)
	} else if report.Multiget != nil {
		return h.handleMultiget(w, report.Multiget)
	}
	return internal.HTTPErrorf(http.StatusBadRequest, "webdav: expected addressbook-query or addressbook-multiget element in REPORT request")
}

func (h *Handler) handleQuery(w http.ResponseWriter, query *addressbookQuery) error {
	var q AddressBookQuery
	if query.Prop != nil {
		var addressData addressDataReq
		if err := query.Prop.Decode(&addressData); err != nil && !internal.IsMissingProp(err) {
			return err
		}
		for _, p := range addressData.Props {
			q.Props = append(q.Props, p.Name)
		}
	}

	aos, err := h.Backend.QueryAddressObjects(&q)
	if err != nil {
		return err
	}

	var resps []internal.Response
	for _, ao := range aos {
		b := backend{h.Backend}
		propfind := internal.Propfind{
			Prop: query.Prop,
			// TODO: Allprop, Propnames
		}
		resp, err := b.propfindAddressObject(&propfind, &ao)
		if err != nil {
			return err
		}
		resps = append(resps, *resp)
	}

	ms := internal.NewMultistatus(resps...)
	return internal.ServeMultistatus(w, ms)
}

func (h *Handler) handleMultiget(w http.ResponseWriter, multiget *addressbookMultiget) error {
	var resps []internal.Response
	for _, href := range multiget.Hrefs {
		ao, err := h.Backend.GetAddressObject(href)
		if err != nil {
			return err // TODO: create internal.Response with error
		}

		b := backend{h.Backend}
		propfind := internal.Propfind{
			Prop: multiget.Prop,
			// TODO: Allprop, Propnames
		}
		resp, err := b.propfindAddressObject(&propfind, ao)
		if err != nil {
			return err
		}
		resps = append(resps, *resp)
	}

	ms := internal.NewMultistatus(resps...)
	return internal.ServeMultistatus(w, ms)
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
		ab, err := b.Backend.AddressBook()
		if err != nil {
			return nil, err
		}

		resp, err := b.propfindAddressBook(propfind, ab)
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

func (b *backend) propfindAddressBook(propfind *internal.Propfind, ab *AddressBook) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
		internal.ResourceTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return internal.NewResourceType(internal.CollectionName, addressBookName), nil
		},
		internal.DisplayNameName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.DisplayName{Name: ab.Name}, nil
		},
		addressBookDescriptionName: func(*internal.RawXMLValue) (interface{}, error) {
			return &addressbookDescription{Description: ab.Description}, nil
		},
		addressBookSupportedAddressData: func(*internal.RawXMLValue) (interface{}, error) {
			return &addressbookSupportedAddressData{
				Types: []addressDataType{
					{ContentType: vcard.MIMEType, Version: "3.0"},
					{ContentType: vcard.MIMEType, Version: "4.0"},
				},
			}, nil
		},
		// TODO: this is a principal property
		addressBookHomeSetName: func(*internal.RawXMLValue) (interface{}, error) {
			return &addressbookHomeSet{Href: "/"}, nil
		},
		// TODO: this should be set on all resources
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: "/"}, nil
		},
	}

	if ab.MaxResourceSize > 0 {
		props[maxResourceSizeName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &maxResourceSize{Size: ab.MaxResourceSize}, nil
		}
	}

	return internal.NewPropfindResponse("/", propfind, props)
}

func (b *backend) propfindAddressObject(propfind *internal.Propfind, ao *AddressObject) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
		internal.GetContentTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetContentType{Type: vcard.MIMEType}, nil
		},
		addressDataName: func(*internal.RawXMLValue) (interface{}, error) {
			var buf bytes.Buffer
			if err := vcard.NewEncoder(&buf).Encode(ao.Card); err != nil {
				return nil, err
			}

			return &addressDataResp{Data: buf.Bytes()}, nil
		},
		// TODO: getlastmodified, getetag
	}

	return internal.NewPropfindResponse(ao.Href, propfind, props)
}

func (b *backend) Put(r *http.Request) error {
	panic("TODO")
}

func (b *backend) Delete(r *http.Request) error {
	panic("TODO")
}

func (b *backend) Mkcol(r *http.Request) error {
	return internal.HTTPErrorf(http.StatusForbidden, "carddav: address book creation unsupported")
}
