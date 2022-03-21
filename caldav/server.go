package caldav

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple calendars

// Backend is a CalDAV server backend.
type Backend interface {
	CalendarHomeSetPath(ctx context.Context) (string, error)
	Calendar(ctx context.Context) (*Calendar, error)
	GetCalendarObject(ctx context.Context, path string, req *CalendarCompRequest) (*CalendarObject, error)
	ListCalendarObjects(ctx context.Context, req *CalendarCompRequest) ([]CalendarObject, error)
	QueryCalendarObjects(ctx context.Context, query *CalendarQuery) ([]CalendarObject, error)

	webdav.UserPrincipalBackend
}

// Handler handles CalDAV HTTP requests. It can be used to create a CalDAV
// server.
type Handler struct {
	Backend Backend
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Backend == nil {
		http.Error(w, "caldav: no backend available", http.StatusInternalServerError)
		return
	}

	principalPath, err := h.Backend.CurrentUserPrincipal(r.Context())
	if err != nil {
		http.Error(w, "caldav: failed to determine current user principal", http.StatusInternalServerError)
		return
	}

	if r.URL.Path == "/.well-known/caldav" {
		http.Redirect(w, r, principalPath, http.StatusMovedPermanently)
		return
	}

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
		return h.handleQuery(r, w, report.Query)
	} else if report.Multiget != nil {
		return h.handleMultiget(w, report.Multiget)
	}
	return internal.HTTPErrorf(http.StatusBadRequest, "caldav: expected calendar-query or calendar-multiget element in REPORT request")
}

func decodeParamFilter(el *paramFilter) (*ParamFilter, error) {
	pf := &ParamFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TextMatch != nil {
			return nil, fmt.Errorf("caldav: failed to parse param-filter: if is-not-defined is provided, text-match can't be provided")
		}
		pf.IsNotDefined = true
	}
	if el.TextMatch != nil {
		pf.TextMatch = &TextMatch{Text: el.TextMatch.Text}
	}
	return pf, nil
}

func decodePropFilter(el *propFilter) (*PropFilter, error) {
	pf := &PropFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TextMatch != nil || el.TimeRange != nil || len(el.ParamFilter) > 0 {
			return nil, fmt.Errorf("caldav: failed to parse prop-filter: if is-not-defined is provided, text-match, time-range, or param-filter can't be provided")
		}
		pf.IsNotDefined = true
	}
	if el.TextMatch != nil {
		pf.TextMatch = &TextMatch{Text: el.TextMatch.Text}
	}
	if el.TimeRange != nil {
		pf.Start = time.Time(el.TimeRange.Start)
		pf.End = time.Time(el.TimeRange.End)
	}
	for _, paramEl := range el.ParamFilter {
		paramFi, err := decodeParamFilter(&paramEl)
		if err != nil {
			return nil, err
		}
		pf.ParamFilter = append(pf.ParamFilter, *paramFi)
	}
	return pf, nil
}

func decodeCompFilter(el *compFilter) (*CompFilter, error) {
	cf := &CompFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TimeRange != nil || len(el.PropFilters) > 0 || len(el.CompFilters) > 0 {
			return nil, fmt.Errorf("caldav: failed to parse comp-filter: if is-not-defined is provided, time-range, prop-filter, or comp-filter can't be provided")
		}
		cf.IsNotDefined = true
	}
	if el.TimeRange != nil {
		cf.Start = time.Time(el.TimeRange.Start)
		cf.End = time.Time(el.TimeRange.End)
	}
	for _, pfEl := range el.PropFilters {
		pf, err := decodePropFilter(&pfEl)
		if err != nil {
			return nil, err
		}
		cf.Props = append(cf.Props, *pf)
	}
	for _, childEl := range el.CompFilters {
		child, err := decodeCompFilter(&childEl)
		if err != nil {
			return nil, err
		}
		cf.Comps = append(cf.Comps, *child)
	}
	return cf, nil
}

func (h *Handler) handleQuery(r *http.Request, w http.ResponseWriter, query *calendarQuery) error {
	var q CalendarQuery
	// TODO: calendar-data in query.Prop
	cf, err := decodeCompFilter(&query.Filter.CompFilter)
	if err != nil {
		return err
	}
	q.CompFilter = *cf

	cos, err := h.Backend.QueryCalendarObjects(r.Context(), &q)
	if err != nil {
		return err
	}

	var resps []internal.Response
	for _, co := range cos {
		b := backend{h.Backend}
		propfind := internal.Propfind{
			Prop:     query.Prop,
			AllProp:  query.AllProp,
			PropName: query.PropName,
		}
		resp, err := b.propfindCalendarObject(&propfind, &co)
		if err != nil {
			return err
		}
		resps = append(resps, *resp)
	}

	ms := internal.NewMultistatus(resps...)

	return internal.ServeMultistatus(w, ms)
}

func (h *Handler) handleMultiget(w http.ResponseWriter, multiget *calendarMultiget) error {
	panic("TODO")
}

type backend struct {
	Backend Backend
}

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	caps = []string{"calendar-access"}

	homeSetPath, err := b.Backend.CalendarHomeSetPath(r.Context())
	if err != nil {
		return nil, nil, err
	}

	principalPath, err := b.Backend.CurrentUserPrincipal(r.Context())
	if err != nil {
		return nil, nil, err
	}

	if r.URL.Path == "/" || r.URL.Path == principalPath || r.URL.Path == homeSetPath {
		return caps, []string{http.MethodOptions, "PROPFIND", "REPORT"}, nil
	}

	var dataReq CalendarCompRequest
	_, err = b.Backend.GetCalendarObject(r.Context(), r.URL.Path, &dataReq)
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
	panic("TODO")
}

func (b *backend) Propfind(r *http.Request, propfind *internal.Propfind, depth internal.Depth) (*internal.Multistatus, error) {
	homeSetPath, err := b.Backend.CalendarHomeSetPath(r.Context())
	if err != nil {
		return nil, err
	}
	principalPath, err := b.Backend.CurrentUserPrincipal(r.Context())
	if err != nil {
		return nil, err
	}

	var resps []internal.Response

	if r.URL.Path == principalPath {
		resp, err := b.propfindUserPrincipal(r.Context(), propfind, homeSetPath)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)
	} else if r.URL.Path == homeSetPath {
		cal, err := b.Backend.Calendar(r.Context())
		if err != nil {
			return nil, err
		}

		resp, err := b.propfindCalendar(r.Context(), propfind, cal)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)

		if depth != internal.DepthZero {
			// TODO
		}
	} else {
		// TODO
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfindUserPrincipal(ctx context.Context, propfind *internal.Propfind, homeSetPath string) (*internal.Response, error) {
	principalPath, err := b.Backend.CurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	props := map[xml.Name]internal.PropfindFunc{
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: principalPath}}, nil
		},
		calendarHomeSetName: func(*internal.RawXMLValue) (interface{}, error) {
			return &calendarHomeSet{Href: internal.Href{Path: homeSetPath}}, nil
		},
	}
	return internal.NewPropfindResponse(principalPath, propfind, props)
}

func (b *backend) propfindCalendar(ctx context.Context, propfind *internal.Propfind, cal *Calendar) (*internal.Response, error) {
	principalPath, err := b.Backend.CurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	props := map[xml.Name]internal.PropfindFunc{
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: principalPath}}, nil
		},
		internal.ResourceTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return internal.NewResourceType(internal.CollectionName, calendarName), nil
		},
		internal.DisplayNameName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.DisplayName{Name: cal.Name}, nil
		},
		supportedCalendarDataName: func(*internal.RawXMLValue) (interface{}, error) {
			return &supportedCalendarData{
				Types: []calendarDataType{
					{ContentType: ical.MIMEType, Version: "2.0"},
				},
			}, nil
		},
		supportedCalendarComponentSetName: func(*internal.RawXMLValue) (interface{}, error) {
			return &supportedCalendarComponentSet{
				Comp: []comp{
					{Name: ical.CompEvent},
				},
			}, nil
		},
	}

	if cal.Description != "" {
		props[calendarDescriptionName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &calendarDescription{Description: cal.Description}, nil
		}
	}

	if cal.MaxResourceSize > 0 {
		props[maxResourceSizeName] = func(*internal.RawXMLValue) (interface{}, error) {
			return &maxResourceSize{Size: cal.MaxResourceSize}, nil
		}
	}

	// TODO: CALDAV:calendar-timezone, CALDAV:supported-calendar-component-set, CALDAV:min-date-time, CALDAV:max-date-time, CALDAV:max-instances, CALDAV:max-attendees-per-instance

	return internal.NewPropfindResponse(cal.Path, propfind, props)
}

func (b *backend) propfindCalendarObject(propfind *internal.Propfind, co *CalendarObject) (*internal.Response, error) {
	panic("TODO")
}

func (b *backend) Proppatch(r *http.Request, update *internal.Propertyupdate) (*internal.Response, error) {
	panic("TODO")
}

func (b *backend) Put(r *http.Request) (*internal.Href, error) {
	panic("TODO")
}

func (b *backend) Delete(r *http.Request) error {
	panic("TODO")
}

func (b *backend) Mkcol(r *http.Request) error {
	panic("TODO")
}

func (b *backend) Copy(r *http.Request, dest *internal.Href, recursive, overwrite bool) (created bool, err error) {
	panic("TODO")
}

func (b *backend) Move(r *http.Request, dest *internal.Href, overwrite bool) (created bool, err error) {
	panic("TODO")
}

// https://datatracker.ietf.org/doc/html/rfc4791#section-5.3.2.1
type PreconditionType string

const (
	PreconditionNoUIDConflict                PreconditionType = "no-uid-conflict"
	PreconditionSupportedCalendarData        PreconditionType = "supported-calendar-data"
	PreconditionSupportedCalendarComponent   PreconditionType = "supported-calendar-component"
	PreconditionValidCalendarData            PreconditionType = "valid-calendar-data"
	PreconditionValidCalendarObjectResource  PreconditionType = "valid-calendar-object-resource"
	PreconditionCalendarCollectionLocationOk PreconditionType = "calendar-collection-location-ok"
	PreconditionMaxResourceSize              PreconditionType = "max-resource-size"
	PreconditionMinDateTime                  PreconditionType = "min-date-time"
	PreconditionMaxDateTime                  PreconditionType = "max-date-time"
	PreconditionMaxInstances                 PreconditionType = "max-instances"
	PreconditionMaxAttendeesPerInstance      PreconditionType = "max-attendees-per-instance"
)

func NewPreconditionError(err PreconditionType) error {
	name := xml.Name{"urn:ietf:params:xml:ns:caldav", string(err)}
	elem := internal.NewRawXMLElement(name, nil, nil)
	return &internal.HTTPError{
		Code: 409,
		Err: &internal.Error{
			Raw: []internal.RawXMLValue{*elem},
		},
	}
}
