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
	GetAddressObject(path string, req *AddressDataRequest) (*AddressObject, error)
	ListAddressObjects(req *AddressDataRequest) ([]AddressObject, error)
	QueryAddressObjects(query *AddressBookQuery) ([]AddressObject, error)
	PutAddressObject(path string, card vcard.Card) (loc string, err error)
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

	if r.URL.Path == "/.well-known/carddav" {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
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
	return internal.HTTPErrorf(http.StatusBadRequest, "carddav: expected addressbook-query or addressbook-multiget element in REPORT request")
}

func decodePropFilter(el *propFilter) (*PropFilter, error) {
	pf := &PropFilter{Name: el.Name, Test: FilterTest(el.Test)}
	if el.IsNotDefined != nil {
		if len(el.TextMatches) > 0 || len(el.Params) > 0 {
			return nil, fmt.Errorf("carddav: failed to parse prop-filter: if is-not-defined is provided, text-match or param-filter can't be provided")
		}
		pf.IsNotDefined = true
	}
	for _, tm := range el.TextMatches {
		pf.TextMatches = append(pf.TextMatches, *decodeTextMatch(&tm))
	}
	for _, paramEl := range el.Params {
		param, err := decodeParamFilter(&paramEl)
		if err != nil {
			return nil, err
		}
		pf.Params = append(pf.Params, *param)
	}
	return pf, nil
}

func decodeParamFilter(el *paramFilter) (*ParamFilter, error) {
	pf := &ParamFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TextMatch != nil {
			return nil, fmt.Errorf("carddav: failed to parse param-filter: if is-not-defined is provided, text-match can't be provided")
		}
		pf.IsNotDefined = true
	}
	if el.TextMatch != nil {
		pf.TextMatch = decodeTextMatch(el.TextMatch)
	}
	return pf, nil
}

func decodeTextMatch(tm *textMatch) *TextMatch {
	return &TextMatch{
		Text:            tm.Text,
		NegateCondition: bool(tm.NegateCondition),
		MatchType:       MatchType(tm.MatchType),
	}
}

func decodeAddressDataReq(addressData *addressDataReq) (*AddressDataRequest, error) {
	if addressData.Allprop != nil && len(addressData.Props) > 0 {
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "carddav: only one of allprop or prop can be specified in address-data")
	}

	req := &AddressDataRequest{AllProp: addressData.Allprop != nil}
	for _, p := range addressData.Props {
		req.Props = append(req.Props, p.Name)
	}
	return req, nil
}

func (h *Handler) handleQuery(w http.ResponseWriter, query *addressbookQuery) error {
	var q AddressBookQuery
	if query.Prop != nil {
		var addressData addressDataReq
		if err := query.Prop.Decode(&addressData); err != nil && !internal.IsNotFound(err) {
			return err
		}
		req, err := decodeAddressDataReq(&addressData)
		if err != nil {
			return err
		}
		q.DataRequest = *req
	}
	q.FilterTest = FilterTest(query.Filter.Test)
	for _, el := range query.Filter.Props {
		pf, err := decodePropFilter(&el)
		if err != nil {
			return &internal.HTTPError{http.StatusBadRequest, err}
		}
		q.PropFilters = append(q.PropFilters, *pf)
	}
	if query.Limit != nil {
		q.Limit = int(query.Limit.NResults)
		if q.Limit <= 0 {
			return internal.ServeMultistatus(w, internal.NewMultistatus())
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
	var dataReq AddressDataRequest
	if multiget.Prop != nil {
		var addressData addressDataReq
		if err := multiget.Prop.Decode(&addressData); err != nil && !internal.IsNotFound(err) {
			return err
		}
		decoded, err := decodeAddressDataReq(&addressData)
		if err != nil {
			return err
		}
		dataReq = *decoded
	}

	var resps []internal.Response
	for _, href := range multiget.Hrefs {
		ao, err := h.Backend.GetAddressObject(href.Path, &dataReq)
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

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	caps = []string{"addressbook"}

	if r.URL.Path == "/" {
		// Note: some clients assume the address book is read-only when
		// DELETE/MKCOL are missing
		return caps, []string{http.MethodOptions, "PROPFIND", "REPORT", "DELETE", "MKCOL"}, nil
	}

	var dataReq AddressDataRequest
	_, err = b.Backend.GetAddressObject(r.URL.Path, &dataReq)
	if httpErr, ok := err.(*internal.HTTPError); ok && httpErr.Code == http.StatusNotFound {
		return caps, []string{http.MethodOptions, http.MethodPut}, nil
	} else if err != nil {
		return nil, nil, err
	}

	return caps, []string{
		http.MethodOptions,
		http.MethodHead,
		http.MethodGet,
		http.MethodPut,
		http.MethodDelete,
		"PROPFIND",
	}, nil
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		return &internal.HTTPError{Code: http.StatusMethodNotAllowed}
	}

	var dataReq AddressDataRequest
	if r.Method != http.MethodHead {
		dataReq.AllProp = true
	}
	ao, err := b.Backend.GetAddressObject(r.URL.Path, &dataReq)
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
	var dataReq AddressDataRequest

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
			aos, err := b.Backend.ListAddressObjects(&dataReq)
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
		ao, err := b.Backend.GetAddressObject(r.URL.Path, &dataReq)
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
		supportedAddressDataName: func(*internal.RawXMLValue) (interface{}, error) {
			return &supportedAddressData{
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
			return &internal.GetETag{ETag: internal.ETag(ao.ETag)}, nil
		}
	}

	return internal.NewPropfindResponse(ao.Path, propfind, props)
}

func (b *backend) Proppatch(r *http.Request, update *internal.Propertyupdate) (*internal.Response, error) {
	// TODO: return a failed Response instead
	// TODO: support PROPPATCH for address books
	return nil, internal.HTTPErrorf(http.StatusForbidden, "carddav: PROPPATCH is unsupported")
}

func (b *backend) Put(r *http.Request) (*internal.Href, error) {
	// TODO: add support for If-None-Match and If-Match

	t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "carddav: malformed Content-Type: %v", err)
	}
	if t != vcard.MIMEType {
		// TODO: send CARDDAV:supported-address-data error
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "carddav: unsupporetd Content-Type %q", t)
	}

	// TODO: check CARDDAV:max-resource-size precondition
	card, err := vcard.NewDecoder(r.Body).Decode()
	if err != nil {
		// TODO: send CARDDAV:valid-address-data error
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "carddav: failed to parse vCard: %v", err)
	}

	// TODO: add support for the CARDDAV:no-uid-conflict error
	loc, err := b.Backend.PutAddressObject(r.URL.Path, card)
	if err != nil {
		return nil, err
	}

	return &internal.Href{Path: loc}, nil
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
