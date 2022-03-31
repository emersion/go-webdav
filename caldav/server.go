package caldav

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/emersion/go-ical"

	"github.com/emersion/go-webdav/internal"
)

// TODO: add support for multiple calendars

// Backend is a CalDAV server backend.
type Backend interface {
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

	if r.URL.Path == "/.well-known/caldav" {
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

	if r.URL.Path == "/" {
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
	var resps []internal.Response
	if r.URL.Path == "/" {
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
	}

	return internal.NewMultistatus(resps...), nil
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
		// TODO: this is a principal property
		calendarHomeSetName: func(*internal.RawXMLValue) (interface{}, error) {
			return &calendarHomeSet{Href: internal.Href{Path: "/"}}, nil
		},
		// TODO: this should be set on all resources
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: "/"}}, nil
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

	return internal.NewPropfindResponse("/", propfind, props)
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
	e := internal.Error{
		Raw: []internal.RawXMLValue{
			*elem,
		},
	}
	return &internal.DAVError{
		Code: 409,
		Msg:  fmt.Sprintf("precondition not met: %s", string(err)),
		Err:  e,
	}
}
