package caldav

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
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

	principalPath := string(webdav.CurrentUserPrincipalPathFromContext(r.Context()))

	if r.URL.Path == "/.well-known/caldav" {
		http.Redirect(w, r, principalPath, http.StatusMovedPermanently)
		return
	}

	switch r.Method {
	case "REPORT":
		if err := h.handleReport(w, r); err != nil {
			internal.ServeError(w, err)
		}
	default:
		b := backend{h.Backend}
		hh := internal.Handler{&b}
		hh.ServeHTTP(w, r)
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

func decodePropFilter(el *propFilter) (*PropFilter, error) {
	pf := &PropFilter{Name: el.Name}
	if el.TextMatch != nil {
		pf.TextMatch = &TextMatch{Text: el.TextMatch.Text}
	}
	// TODO: IsNotDefined, TimeRange
	return pf, nil
}

func decodeCompFilter(el *compFilter) (*CompFilter, error) {
	cf := &CompFilter{Name: el.Name}
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
	// TODO: IsNotDefined
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

	principalPath := string(webdav.CurrentUserPrincipalPathFromContext(r.Context()))

	if r.URL.Path == "/" || r.URL.Path == principalPath || r.URL.Path == homeSetPath {
		return caps, []string{http.MethodOptions, "PROPFIND", "REPORT"}, nil
	}

	if !strings.HasPrefix(r.URL.Path, homeSetPath) {
		return nil, nil, &internal.HTTPError{Code: http.StatusMethodNotAllowed}
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
	principalPath := string(webdav.CurrentUserPrincipalPathFromContext(r.Context()))
	homeSetPath, err := b.Backend.CalendarHomeSetPath(r.Context())
	if err != nil {
		return nil, err
	}
	isResourceRequest := !strings.HasPrefix(r.URL.Path, homeSetPath)

	var resps []internal.Response

	resp, err := b.propfindCommon(propfind, r.URL.Path, principalPath, homeSetPath)
	if err != nil {
		return nil, err
	}
	resps = append(resps, *resp)

	if r.URL.Path == homeSetPath {
		cal, err := b.Backend.Calendar(r.Context())
		if err != nil {
			return nil, err
		}

		resp, err := b.propfindCalendar(propfind, cal)
		if err != nil {
			return nil, err
		}
		resps = append(resps, *resp)

		if depth != internal.DepthZero {
			// TODO
		}
	} else if isResourceRequest {
		// TODO
	}

	return internal.NewMultistatus(resps...), nil
}

func (b *backend) propfindCommon(propfind *internal.Propfind, path, principalPath, homeSetPath string) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
		calendarHomeSetName: func(*internal.RawXMLValue) (interface{}, error) {
			return &calendarHomeSet{Href: internal.Href{Path: homeSetPath}}, nil
		},
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: principalPath}}, nil
		},
	}
	return internal.NewPropfindResponse(path, propfind, props)
}

func (b *backend) propfindCalendar(propfind *internal.Propfind, cal *Calendar) (*internal.Response, error) {
	props := map[xml.Name]internal.PropfindFunc{
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
