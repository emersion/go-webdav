package carddav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"strconv"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple address books

type PutAddressObjectOptions struct {
	// IfNoneMatch indicates that the client does not want to overwrite
	// an existing resource.
	IfNoneMatch bool
	// IfMatch provides the ETag of the resource that the client intends
	// to overwrite, can be ""
	IfMatch string
}

type ResourceType int

const (
	ResourceTypeUserPrincipal      ResourceType = 1 << iota
	ResourceTypeAddressBookHomeSet ResourceType = 1 << iota
	ResourceTypeAddressBook        ResourceType = 1 << iota
	ResourceTypeAddressObject      ResourceType = 1 << iota
)

// Backend is a CardDAV server backend.
type Backend interface {
	CardDAVResourceType(ctx context.Context, path string) (ResourceType, error)
	AddressbookHomeSetPath(ctx context.Context) (string, error)
	AddressBook(ctx context.Context) (*AddressBook, error)
	GetAddressObject(ctx context.Context, path string, req *AddressDataRequest) (*AddressObject, error)
	ListAddressObjects(ctx context.Context, req *AddressDataRequest) ([]AddressObject, error)
	QueryAddressObjects(ctx context.Context, query *AddressBookQuery) ([]AddressObject, error)
	PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *PutAddressObjectOptions) (loc string, err error)
	DeleteAddressObject(ctx context.Context, path string) error

	webdav.UserPrincipalBackend
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
		principalPath, err := h.Backend.CurrentUserPrincipal(r.Context())
		if err != nil {
			http.Error(w, "carddav: failed to determine current user principal", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, principalPath, http.StatusPermanentRedirect)
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
		return h.handleQuery(r.Context(), w, report.Query)
	} else if report.Multiget != nil {
		return h.handleMultiget(r.Context(), w, report.Multiget)
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

func (h *Handler) handleQuery(ctx context.Context, w http.ResponseWriter, query *addressbookQuery) error {
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
			return internal.ServeMultiStatus(w, internal.NewMultiStatus())
		}
	}

	aos, err := h.Backend.QueryAddressObjects(ctx, &q)
	if err != nil {
		return err
	}

	var resps []internal.Response
	for _, ao := range aos {
		b := backend{h.Backend}
		propfind := internal.PropFind{
			Prop:     query.Prop,
			AllProp:  query.AllProp,
			PropName: query.PropName,
		}
		resp, err := b.propFindAddressObject(ctx, &propfind, &ao)
		if err != nil {
			return err
		}
		resps = append(resps, *resp)
	}

	ms := internal.NewMultiStatus(resps...)
	return internal.ServeMultiStatus(w, ms)
}

func (h *Handler) handleMultiget(ctx context.Context, w http.ResponseWriter, multiget *addressbookMultiget) error {
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
		ao, err := h.Backend.GetAddressObject(ctx, href.Path, &dataReq)
		if err != nil {
			resp := internal.NewErrorResponse(href.Path, err)
			resps = append(resps, *resp)
			continue
		}

		b := backend{h.Backend}
		propfind := internal.PropFind{
			Prop:     multiget.Prop,
			AllProp:  multiget.AllProp,
			PropName: multiget.PropName,
		}
		resp, err := b.propFindAddressObject(ctx, &propfind, ao)
		if err != nil {
			return err
		}
		resps = append(resps, *resp)
	}

	ms := internal.NewMultiStatus(resps...)
	return internal.ServeMultiStatus(w, ms)
}

type backend struct {
	Backend Backend
}

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	caps = []string{"addressbook"}

	homeSetPath, err := b.Backend.AddressbookHomeSetPath(r.Context())
	if err != nil {
		return nil, nil, err
	}

	principalPath, err := b.Backend.CurrentUserPrincipal(r.Context())
	if err != nil {
		return nil, nil, err
	}

	if r.URL.Path == "/" || r.URL.Path == principalPath || r.URL.Path == homeSetPath {
		// Note: some clients assume the address book is read-only when
		// DELETE/MKCOL are missing
		return caps, []string{http.MethodOptions, "PROPFIND", "REPORT", "DELETE", "MKCOL"}, nil
	}

	var dataReq AddressDataRequest
	_, err = b.Backend.GetAddressObject(r.Context(), r.URL.Path, &dataReq)
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
	var dataReq AddressDataRequest
	if r.Method != http.MethodHead {
		dataReq.AllProp = true
	}
	ao, err := b.Backend.GetAddressObject(r.Context(), r.URL.Path, &dataReq)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", vcard.MIMEType)
	if ao.ContentLength > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(ao.ContentLength, 10))
	}
	if ao.ETag != "" {
		w.Header().Set("ETag", internal.ETag(ao.ETag).String())
	}
	if !ao.ModTime.IsZero() {
		w.Header().Set("Last-Modified", ao.ModTime.UTC().Format(http.TimeFormat))
	}

	if r.Method != http.MethodHead {
		return vcard.NewEncoder(w).Encode(ao.Card)
	}
	return nil
}

func (b *backend) PropFind(r *http.Request, propfind *internal.PropFind, depth internal.Depth) (*internal.MultiStatus, error) {
	var resps []internal.Response

	// Get the response for this very path
	resp, err := b.propFindResponse(r.Context(), propfind, r.URL.Path)
	if err != nil {
		return nil, err
	}
	resps = append(resps, *resp)

	// Depth handling...
	if depth != internal.DepthZero {
		// Figure out the type of resource at the _current_ path
		resType, err := b.Backend.CardDAVResourceType(r.Context(), r.URL.Path)
		if err != nil {
			return nil, err
		}

		if (resType & ResourceTypeUserPrincipal) > 0 {
			// Only handle depth here if user principal path != home set path
			// If they are the same, the next if statement will handle it
			if resType&ResourceTypeAddressBookHomeSet == 0 {
				homeSetPath, err := b.Backend.AddressbookHomeSetPath(r.Context())
				if err != nil {
					return nil, err
				}
				resp, err := b.propFindResponse(r.Context(), propfind, homeSetPath)
				resps = append(resps, *resp)
			}
			// TODO handle depth infinity...
		}

		if (resType & ResourceTypeAddressBookHomeSet) > 0 {
			// If the home set path is also the addressbook path, the address objects will be handled below.
			// But if not _and_ depth is infinity, add address objects.
			if (resType&ResourceTypeAddressBook) == 0 && depth == internal.DepthInfinity {
				aoResps, err := b.propFindAllAddressObjects(r.Context(), propfind)
				if err != nil {
					return nil, err
				}
				resps = append(resps, aoResps...)
			}
			// Add address book if it is a real child of this path
			if (resType & ResourceTypeAddressBook) == 0 {
				ab, err := b.Backend.AddressBook(r.Context())
				if err != nil {
					return nil, err
				}
				resp, err := b.propFindResponse(r.Context(), propfind, ab.Path)
				if err != nil {
					return nil, err
				}
				resps = append(resps, *resp)
			}
		}
		if (resType & ResourceTypeAddressBook) > 0 {
			aoResps, err := b.propFindAllAddressObjects(r.Context(), propfind)
			if err != nil {
				return nil, err
			}
			resps = append(resps, aoResps...)
		}
	}

	return internal.NewMultiStatus(resps...), nil
}

func (b *backend) propFindResponse(ctx context.Context, propfind *internal.PropFind, path string) (*internal.Response, error) {
	resType, err := b.Backend.CardDAVResourceType(ctx, path)
	if err != nil {
		return nil, err
	}

	// This is the only type that cannot overlap with others
	if (resType & ResourceTypeAddressObject) > 0 {
		var dataReq AddressDataRequest
		ao, err := b.Backend.GetAddressObject(ctx, path, &dataReq)
		if err != nil {
			return nil, err
		}
		return b.propFindAddressObject(ctx, propfind, ao)
	}

	props := make(map[xml.Name]internal.PropFindFunc)

	// Order is important. If user principal path, home set path, and
	// address book path are all the same (a supported configuration), we
	// want the resource type to be the one for the address book.
	if (resType & ResourceTypeUserPrincipal) > 0 {
		homeSetPath, err := b.Backend.AddressbookHomeSetPath(ctx)
		if err != nil {
			return nil, err
		}
		b.propFindAddPropsForUserPrincipal(ctx, props, path, homeSetPath)
	}
	if (resType & ResourceTypeAddressBookHomeSet) > 0 {
		b.propFindAddPropsForHomeSet(ctx, props)
	}
	if (resType & ResourceTypeAddressBook) > 0 {
		ab, err := b.Backend.AddressBook(ctx)
		if err != nil {
			return nil, err
		}
		b.propFindAddPropsForAddressBook(ctx, props, ab)
	}

	return internal.NewPropFindResponse(path, propfind, props)
}

func (b *backend) propFindAddPropsForUserPrincipal(ctx context.Context, props map[xml.Name]internal.PropFindFunc, principalPath, homeSetPath string) {
	props[internal.CurrentUserPrincipalName] = func(*internal.RawXMLValue) (interface{}, error) {
		return &internal.CurrentUserPrincipal{Href: internal.Href{Path: principalPath}}, nil
	}
	props[addressBookHomeSetName] = func(*internal.RawXMLValue) (interface{}, error) {
		return &addressbookHomeSet{Href: internal.Href{Path: homeSetPath}}, nil
	}
	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		return internal.NewResourceType(internal.CollectionName), nil
	}
}

func (b *backend) propFindAddPropsForHomeSet(ctx context.Context, props map[xml.Name]internal.PropFindFunc) {
	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		return internal.NewResourceType(internal.CollectionName), nil
	}
}

func (b *backend) propFindAddPropsForAddressBook(ctx context.Context, props map[xml.Name]internal.PropFindFunc, ab *AddressBook) {
	props[internal.CurrentUserPrincipalName] = func(*internal.RawXMLValue) (interface{}, error) {
		path, err := b.Backend.CurrentUserPrincipal(ctx)
		if err != nil {
			return nil, err
		}
		return &internal.CurrentUserPrincipal{Href: internal.Href{Path: path}}, nil
	}
	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		return internal.NewResourceType(internal.CollectionName, addressBookName), nil
	}
	props[internal.DisplayNameName] = func(*internal.RawXMLValue) (interface{}, error) {
		return &internal.DisplayName{Name: ab.Name}, nil
	}
	props[addressBookDescriptionName] = func(*internal.RawXMLValue) (interface{}, error) {
		return &addressbookDescription{Description: ab.Description}, nil
	}
	props[supportedAddressDataName] = func(*internal.RawXMLValue) (interface{}, error) {
		return &supportedAddressData{
			Types: []addressDataType{
				{ContentType: vcard.MIMEType, Version: "3.0"},
				{ContentType: vcard.MIMEType, Version: "4.0"},
			},
		}, nil
	}

	if ab.MaxResourceSize > 0 {
		props[maxResourceSizeName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &maxResourceSize{Size: ab.MaxResourceSize}, nil
		}
	}
}

func (b *backend) propFindAllAddressObjects(ctx context.Context, propfind *internal.PropFind) ([]internal.Response, error) {
	var dataReq AddressDataRequest
	aos, err := b.Backend.ListAddressObjects(ctx, &dataReq)
	if err != nil {
		return nil, err
	}

	var resps []internal.Response
	for _, ao := range aos {
		resp, err := b.propFindAddressObject(ctx, propfind, &ao)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)
	}
	return resps, nil
}

func (b *backend) propFindAddressObject(ctx context.Context, propfind *internal.PropFind, ao *AddressObject) (*internal.Response, error) {
	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			path, err := b.Backend.CurrentUserPrincipal(ctx)
			if err != nil {
				return nil, err
			}
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: path}}, nil
		},
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

	if ao.ContentLength > 0 {
		props[internal.GetContentLengthName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.GetContentLength{Length: ao.ContentLength}, nil
		}
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

	return internal.NewPropFindResponse(ao.Path, propfind, props)
}

func (b *backend) PropPatch(r *http.Request, update *internal.PropertyUpdate) (*internal.Response, error) {
	homeSetPath, err := b.Backend.AddressbookHomeSetPath(r.Context())
	if err != nil {
		return nil, err
	}

	resp := internal.NewOKResponse(r.URL.Path)

	if r.URL.Path == homeSetPath {
		// TODO: support PROPPATCH for address books
		for _, prop := range update.Remove {
			emptyVal := internal.NewRawXMLElement(prop.Prop.XMLName, nil, nil)
			if err := resp.EncodeProp(http.StatusNotImplemented, emptyVal); err != nil {
				return nil, err
			}
		}
		for _, prop := range update.Set {
			emptyVal := internal.NewRawXMLElement(prop.Prop.XMLName, nil, nil)
			if err := resp.EncodeProp(http.StatusNotImplemented, emptyVal); err != nil {
				return nil, err
			}
		}
	} else {
		for _, prop := range update.Remove {
			emptyVal := internal.NewRawXMLElement(prop.Prop.XMLName, nil, nil)
			if err := resp.EncodeProp(http.StatusMethodNotAllowed, emptyVal); err != nil {
				return nil, err
			}
		}
		for _, prop := range update.Set {
			emptyVal := internal.NewRawXMLElement(prop.Prop.XMLName, nil, nil)
			if err := resp.EncodeProp(http.StatusMethodNotAllowed, emptyVal); err != nil {
				return nil, err
			}
		}
	}

	return resp, nil
}

func (b *backend) Put(r *http.Request) (*internal.Href, error) {
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm != "*" {
		return nil, internal.HTTPErrorf(http.StatusBadRequest, "invalid value for If-None-Match header")
	}

	opts := PutAddressObjectOptions{
		IfNoneMatch: r.Header.Get("If-None-Match") == "*",
		IfMatch:     r.Header.Get("If-Match"),
	}

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
	loc, err := b.Backend.PutAddressObject(r.Context(), r.URL.Path, card, &opts)
	if err != nil {
		return nil, err
	}

	return &internal.Href{Path: loc}, nil
}

func (b *backend) Delete(r *http.Request) error {
	return b.Backend.DeleteAddressObject(r.Context(), r.URL.Path)
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

// https://tools.ietf.org/rfcmarkup?doc=6352#section-6.3.2.1
type PreconditionType string

const (
	PreconditionNoUIDConflict        PreconditionType = "no-uid-conflict"
	PreconditionSupportedAddressData PreconditionType = "supported-address-data"
	PreconditionValidAddressData     PreconditionType = "valid-address-data"
	PreconditionMaxResourceSize      PreconditionType = "max-resource-size"
)

func NewPreconditionError(err PreconditionType) error {
	name := xml.Name{"urn:ietf:params:xml:ns:carddav", string(err)}
	elem := internal.NewRawXMLElement(name, nil, nil)
	return &internal.HTTPError{
		Code: 409,
		Err: &internal.Error{
			Raw: []internal.RawXMLValue{*elem},
		},
	}
}
