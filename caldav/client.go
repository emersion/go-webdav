package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

var calendarProps = []xml.Name{
	internal.ResourceTypeName,
	internal.DisplayNameName,
	calendarDescriptionName,
	maxResourceSizeName,
	supportedCalendarComponentSetName,
}

// DiscoverContextURL performs a DNS-based CardDAV service discovery as
// described in RFC 6352 section 11. It returns the URL to the CardDAV server.
func DiscoverContextURL(ctx context.Context, domain string) (string, error) {
	return internal.DiscoverContextURL(ctx, "caldav", domain)
}

// Client provides access to a remote CardDAV server.
type Client struct {
	*webdav.Client

	ic *internal.Client
}

func NewClient(c webdav.HTTPClient, endpoint string) (*Client, error) {
	wc, err := webdav.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	ic, err := internal.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{wc, ic}, nil
}

// Get calendar information without loading all the events once the path of the calendar is known.
func (c *Client) FindCalendar(ctx context.Context, path string) (*Calendar, error) {
	propfind := internal.NewPropNamePropFind(calendarProps...)
	resp, err := c.ic.PropFindFlat(ctx, path, propfind)
	if err != nil {
		return nil, err
	}

	calendar, err := decodeCalendar(path, resp)
	if err != nil {
		return nil, err
	}

	return calendar, nil
}

func (c *Client) FindCalendarHomeSet(ctx context.Context, principal string) (string, error) {
	propfind := internal.NewPropNamePropFind(calendarHomeSetName)
	resp, err := c.ic.PropFindFlat(ctx, principal, propfind)
	if err != nil {
		return "", err
	}

	var prop calendarHomeSet
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}

	return prop.Href.Path, nil
}

func (c *Client) FindCalendars(ctx context.Context, calendarHomeSet string) ([]Calendar, error) {
	propfind := internal.NewPropNamePropFind(calendarProps...)
	ms, err := c.ic.PropFind(ctx, calendarHomeSet, internal.DepthOne, propfind)
	if err != nil {
		return nil, err
	}

	l := make([]Calendar, 0, len(ms.Responses))
	errs := make([]error, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
		if err != nil {
			errs = append(errs, err)
			continue
		}

		var resType internal.ResourceType
		if err := resp.DecodeProp(&resType); err != nil {
			return nil, err
		}
		if !resType.Is(calendarName) {
			continue
		}

		calendar, err := decodeCalendar(path, &resp)
		if err != nil {
			return nil, err
		}

		l = append(l, *calendar)
	}

	return l, errors.Join(errs...)
}

func decodeCalendar(path string, resp *internal.Response) (*Calendar, error) {
	var desc calendarDescription
	if err := resp.DecodeProp(&desc); err != nil && !internal.IsNotFound(err) {
		return nil, err
	}

	var dispName internal.DisplayName
	if err := resp.DecodeProp(&dispName); err != nil && !internal.IsNotFound(err) {
		return nil, err
	}

	var maxResSize maxResourceSize
	if err := resp.DecodeProp(&maxResSize); err != nil && !internal.IsNotFound(err) {
		return nil, err
	}
	if maxResSize.Size < 0 {
		return nil, fmt.Errorf("carddav: max-resource-size must be a positive integer")
	}

	var supportedCompSet supportedCalendarComponentSet
	if err := resp.DecodeProp(&supportedCompSet); err != nil && !internal.IsNotFound(err) {
		return nil, err
	}

	compNames := make([]string, 0, len(supportedCompSet.Comp))
	for _, comp := range supportedCompSet.Comp {
		compNames = append(compNames, comp.Name)
	}

	return &Calendar{
		Path:                  path,
		Name:                  dispName.Name,
		Description:           desc.Description,
		MaxResourceSize:       maxResSize.Size,
		SupportedComponentSet: compNames,
	}, nil
}

func encodeCalendarCompReq(c *CalendarCompRequest) (*comp, error) {
	if c == nil || (c.Name == "" && !c.AllProps && !c.AllComps && len(c.Props) == 0 && len(c.Comps) == 0) {
		return nil, nil
	}

	encoded := comp{Name: c.Name}

	if c.AllProps {
		encoded.Allprop = &struct{}{}
	}
	for _, name := range c.Props {
		encoded.Prop = append(encoded.Prop, prop{Name: name})
	}

	if c.AllComps {
		encoded.Allcomp = &struct{}{}
	}
	for _, child := range c.Comps {
		encodedChild, err := encodeCalendarCompReq(&child)
		if err != nil {
			return nil, err
		}
		encoded.Comp = append(encoded.Comp, *encodedChild)
	}

	return &encoded, nil
}

func encodeCalendarReq(c *CalendarCompRequest) (*internal.Prop, error) {
	compReq, err := encodeCalendarCompReq(c)
	if err != nil {
		return nil, err
	}

	expandReq := encodeExpandRequest(c.Expand)

	calDataReq := calendarDataReq{Comp: compReq, Expand: expandReq}

	getLastModReq := internal.NewRawXMLElement(internal.GetLastModifiedName, nil, nil)
	getETagReq := internal.NewRawXMLElement(internal.GetETagName, nil, nil)
	return internal.EncodeProp(&calDataReq, getLastModReq, getETagReq)
}

func encodeCompFilter(filter *CompFilter) *compFilter {
	encoded := compFilter{Name: filter.Name}
	if !filter.Start.IsZero() || !filter.End.IsZero() {
		encoded.TimeRange = &timeRange{
			Start: dateWithUTCTime(filter.Start),
			End:   dateWithUTCTime(filter.End),
		}
	}
	for _, child := range filter.Comps {
		encoded.CompFilters = append(encoded.CompFilters, *encodeCompFilter(&child))
	}
	for _, pf := range filter.Props {
		encoded.PropFilters = append(encoded.PropFilters, *encodePropFilter(&pf))
	}
	return &encoded
}

func encodePropFilter(filter *PropFilter) *propFilter {
	encoded := propFilter{Name: filter.Name}
	if !filter.Start.IsZero() || !filter.End.IsZero() {
		encoded.TimeRange = &timeRange{
			Start: dateWithUTCTime(filter.Start),
			End:   dateWithUTCTime(filter.End),
		}
	}
	encoded.TextMatch = encodeTextMatch(filter.TextMatch)
	for _, pf := range filter.ParamFilter {
		encoded.ParamFilter = append(encoded.ParamFilter, encodeParamFilter(pf))
	}
	return &encoded
}

func encodeParamFilter(pf ParamFilter) paramFilter {
	encoded := paramFilter{
		Name:      pf.Name,
		TextMatch: encodeTextMatch(pf.TextMatch),
	}
	return encoded
}

func encodeTextMatch(tm *TextMatch) *textMatch {
	if tm == nil {
		return nil
	}

	encoded := &textMatch{
		Text:            tm.Text,
		NegateCondition: negateCondition(tm.NegateCondition),
	}
	return encoded
}

func encodeExpandRequest(e *CalendarExpandRequest) *expand {
	if e == nil {
		return nil
	}
	encoded := expand{
		Start: dateWithUTCTime(e.Start),
		End:   dateWithUTCTime(e.End),
	}
	return &encoded
}

func decodeCalendarObjectList(ms *internal.MultiStatus) ([]CalendarObject, error) {
	addrs := make([]CalendarObject, 0, len(ms.Responses))
	errs := make([]error, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
		if err != nil {
			errs = append(errs, err)
			continue
		}

		var calData calendarDataResp
		if err := resp.DecodeProp(&calData); err != nil {
			return nil, err
		}

		var getLastMod internal.GetLastModified
		if err := resp.DecodeProp(&getLastMod); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var getETag internal.GetETag
		if err := resp.DecodeProp(&getETag); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var getContentLength internal.GetContentLength
		if err := resp.DecodeProp(&getContentLength); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		r := bytes.NewReader(calData.Data)
		data, err := ical.NewDecoder(r).Decode()
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, CalendarObject{
			Path:          path,
			ModTime:       time.Time(getLastMod.LastModified),
			ContentLength: getContentLength.Length,
			ETag:          string(getETag.ETag),
			Data:          data,
		})
	}

	return addrs, errors.Join(errs...)
}

func (c *Client) QueryCalendar(ctx context.Context, calendar string, query *CalendarQuery) ([]CalendarObject, error) {
	propReq, err := encodeCalendarReq(&query.CompRequest)
	if err != nil {
		return nil, err
	}

	calendarQuery := calendarQuery{Prop: propReq}
	calendarQuery.Filter.CompFilter = *encodeCompFilter(&query.CompFilter)
	req, err := c.ic.NewXMLRequest("REPORT", calendar, &calendarQuery)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Depth", "1")

	ms, err := c.ic.DoMultiStatus(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return decodeCalendarObjectList(ms)
}

func (c *Client) MultiGetCalendar(ctx context.Context, path string, multiGet *CalendarMultiGet) ([]CalendarObject, error) {
	propReq, err := encodeCalendarReq(&multiGet.CompRequest)
	if err != nil {
		return nil, err
	}

	calendarMultiget := calendarMultiget{Prop: propReq}

	if len(multiGet.Paths) == 0 {
		href := internal.Href{Path: path}
		calendarMultiget.Hrefs = []internal.Href{href}
	} else {
		calendarMultiget.Hrefs = make([]internal.Href, len(multiGet.Paths))
		for i, p := range multiGet.Paths {
			calendarMultiget.Hrefs[i] = internal.Href{Path: p}
		}
	}

	req, err := c.ic.NewXMLRequest("REPORT", path, &calendarMultiget)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Depth", "1")

	ms, err := c.ic.DoMultiStatus(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return decodeCalendarObjectList(ms)
}

func populateCalendarObject(co *CalendarObject, h http.Header) error {
	if loc := h.Get("Location"); loc != "" {
		u, err := url.Parse(loc)
		if err != nil {
			return err
		}
		co.Path = u.Path
	}
	if etag := h.Get("ETag"); etag != "" {
		etag, err := strconv.Unquote(etag)
		if err != nil {
			return err
		}
		co.ETag = etag
	}
	if contentLength := h.Get("Content-Length"); contentLength != "" {
		n, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return err
		}
		co.ContentLength = n
	}
	if lastModified := h.Get("Last-Modified"); lastModified != "" {
		t, err := http.ParseTime(lastModified)
		if err != nil {
			return err
		}
		co.ModTime = t
	}

	return nil
}

func (c *Client) GetCalendarObject(ctx context.Context, path string) (*CalendarObject, error) {
	req, err := c.ic.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", ical.MIMEType)

	resp, err := c.ic.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(mediaType, ical.MIMEType) {
		return nil, fmt.Errorf("caldav: expected Content-Type %q, got %q", ical.MIMEType, mediaType)
	}

	cal, err := ical.NewDecoder(resp.Body).Decode()
	if err != nil {
		return nil, err
	}

	co := &CalendarObject{
		Path: resp.Request.URL.Path,
		Data: cal,
	}
	if err := populateCalendarObject(co, resp.Header); err != nil {
		return nil, err
	}
	return co, nil
}

func (c *Client) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar) (*CalendarObject, error) {
	// TODO: add support for If-None-Match and If-Match

	// TODO: some servers want a Content-Length header, so we can't stream the
	// request body here. See the Radicale issue:
	// https://github.com/Kozea/Radicale/issues/1016

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}

	req, err := c.ic.NewRequest(http.MethodPut, path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ical.MIMEType)

	resp, err := c.ic.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	co := &CalendarObject{Path: path}
	if err := populateCalendarObject(co, resp.Header); err != nil {
		return nil, err
	}
	return co, nil
}

// SyncCollection performs a collection synchronization operation on the
// specified resource, as defined in RFC 6578.
func (c *Client) SyncCollection(ctx context.Context, path string, query *SyncQuery) (*SyncResponse, error) {
	var limit *internal.Limit
	if query.Limit > 0 {
		limit = &internal.Limit{NResults: uint(query.Limit)}
	}

	compReq, err := encodeCalendarReq(&query.CompRequest)
	if err != nil {
		return nil, err
	}

	ms, err := c.ic.SyncCollection(ctx, path, query.SyncToken, internal.DepthOne, limit, compReq)
	if err != nil {
		return nil, err
	}

	ret := &SyncResponse{SyncToken: ms.SyncToken}
	var errs []error
	for _, resp := range ms.Responses {
		p, err := resp.Path()
		if err != nil {
			var httpErr *internal.HTTPError
			hasStatus := errors.As(err, &httpErr)
			if !hasStatus || httpErr.Code != http.StatusNotFound {
				errs = append(errs, err)
				continue
			}
			ret.Deleted = append(ret.Deleted, p)
			continue
		}

		if p == path || path == fmt.Sprintf("%s/", p) {
			continue
		}

		var getLastMod internal.GetLastModified
		if err := resp.DecodeProp(&getLastMod); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var getETag internal.GetETag
		if err := resp.DecodeProp(&getETag); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		o := CalendarObject{
			Path:    p,
			ModTime: time.Time(getLastMod.LastModified),
			ETag:    string(getETag.ETag),
		}
		ret.Updated = append(ret.Updated, o)
	}

	return ret, errors.Join(errs...)

}
