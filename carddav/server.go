package carddav

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple address books

// Backend is a CardDAV server backend.
type Backend interface {
	AddressBook() (*AddressBook, error)
	GetAddressObject(path string) (*AddressObject, error)
	ListAddressObjects() ([]AddressObject, error)
	QueryAddressObjects(query *AddressBookQuery) ([]AddressObject, error)
	PutAddressObject(path string, card vcard.Card) error
	DeleteAddressObject(path string) error
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
		if addressData.Allprop != nil && len(addressData.Props) > 0 {
			return internal.HTTPErrorf(http.StatusBadRequest, "carddav: only one of allprop or prop can be specified in address-data")
		}
		q.AllProp = addressData.Allprop != nil
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
			Prop:     query.Prop,
			AllProp:  query.AllProp,
			PropName: query.PropName,
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
		// TODO: only get a subset of the vCard fields, depending on the
		// multiget query
		ao, err := h.Backend.GetAddressObject(href.Path)
		if err != nil {
			return err // TODO: create internal.Response with error
		}

		b := backend{h.Backend}
		propfind := internal.Propfind{
			Prop:     multiget.Prop,
			AllProp:  multiget.AllProp,
			PropName: multiget.PropName,
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
			return &addressbookHomeSet{Href: internal.Href{Path: "/"}}, nil
		},
		// TODO: this should be set on all resources
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: "/"}}, nil
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
		// TODO: address-data can only be used in REPORT requests
		addressDataName: func(*internal.RawXMLValue) (interface{}, error) {
			var buf bytes.Buffer
			if err := vcard.NewEncoder(&buf).Encode(ao.Card); err != nil {
				return nil, err
			}

			return &addressDataResp{Data: buf.Bytes()}, nil
		},
	}

	if !ao.ModTime.IsZero() {
		props[internal.GetLastModifiedName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetLastModified{LastModified: internal.Time(ao.ModTime)}, nil
		}
	}

	if ao.ETag != "" {
		props[internal.GetETagName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetETag{ETag: fmt.Sprintf("%q", ao.ETag)}, nil
		}
	}

	return internal.NewPropfindResponse(ao.Path, propfind, props)
}

func (b *backend) Proppatch(r *http.Request, update *internal.Propertyupdate) (*internal.Response, error) {
	// TODO: return a failed Response instead
	// TODO: support PROPPATCH for address books
	return nil, internal.HTTPErrorf(http.StatusForbidden, "carddav: PROPPATCH is unsupported")
}

func (b *backend) Put(r *http.Request) error {
	// TODO: add support for If-None-Match

	t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return internal.HTTPErrorf(http.StatusBadRequest, "carddav: malformed Content-Type: %v", err)
	}
	if t != vcard.MIMEType {
		// TODO: send CARDDAV:supported-address-data error
		return internal.HTTPErrorf(http.StatusBadRequest, "carddav: unsupporetd Content-Type %q", t)
	}

	// TODO: check CARDDAV:max-resource-size precondition
	card, err := vcard.NewDecoder(r.Body).Decode()
	if err != nil {
		// TODO: send CARDDAV:valid-address-data error
		return internal.HTTPErrorf(http.StatusBadRequest, "carddav: failed to parse vCard: %v", err)
	}

	// TODO: add support for the CARDDAV:no-uid-conflict error
	return b.Backend.PutAddressObject(r.URL.Path, card)
}

func (b *backend) Delete(r *http.Request) error {
	return b.Backend.DeleteAddressObject(r.URL.Path)
}

func (b *backend) Mkcol(r *http.Request) error {
	return internal.HTTPErrorf(http.StatusForbidden, "carddav: address book creation unsupported")
}

func (b *backend) Copy(r *http.Request, dest *internal.Href, recursive, overwrite bool) (created bool, err error) {
	panic("TODO")
}

func (b *backend) Move(r *http.Request, dest *internal.Href, overwrite bool) (created bool, err error) {
	panic("TODO")
}
