package caldav

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/internal"
)

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

func (c *Client) FindCalendarHomeSet(principal string) (string, error) {
	propfind := internal.NewPropNamePropfind(calendarHomeSetName)
	resp, err := c.ic.PropfindFlat(principal, propfind)
	if err != nil {
		return "", err
	}

	var prop calendarHomeSet
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}

	return prop.Href.Path, nil
}

func (c *Client) FindCalendars(calendarHomeSet string) ([]Calendar, error) {
	propfind := internal.NewPropNamePropfind(
		internal.ResourceTypeName,
		internal.DisplayNameName,
		calendarDescriptionName,
		maxResourceSizeName,
	)
	ms, err := c.ic.Propfind(calendarHomeSet, internal.DepthOne, propfind)
	if err != nil {
		return nil, err
	}

	l := make([]Calendar, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
		if err != nil {
			return nil, err
		}

		var resType internal.ResourceType
		if err := resp.DecodeProp(&resType); err != nil {
			return nil, err
		}
		if !resType.Is(calendarName) {
			continue
		}

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

		l = append(l, Calendar{
			Path:            path,
			Name:            dispName.Name,
			Description:     desc.Description,
			MaxResourceSize: maxResSize.Size,
		})
	}

	return l, nil
}

func encodeCalendarCompReq(c *CalendarCompRequest) (*comp, error) {
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

	calDataReq := calendarDataReq{Comp: compReq}

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
	return &encoded
}

func decodeCalendarObjectList(ms *internal.Multistatus) ([]CalendarObject, error) {
	addrs := make([]CalendarObject, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		path, err := resp.Path()
		if err != nil {
			return nil, err
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

		r := bytes.NewReader(calData.Data)
		data, err := ical.NewDecoder(r).DecodeCalendar()
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, CalendarObject{
			Path:    path,
			ModTime: time.Time(getLastMod.LastModified),
			ETag:    string(getETag.ETag),
			Data:    data,
		})
	}

	return addrs, nil
}

func (c *Client) QueryCalendar(calendar string, query *CalendarQuery) ([]CalendarObject, error) {
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

	ms, err := c.ic.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	return decodeCalendarObjectList(ms)
}

func populateCalendarObject(co *CalendarObject, resp *http.Response) error {
	if loc := resp.Header.Get("Location"); loc != "" {
		u, err := url.Parse(loc)
		if err != nil {
			return err
		}
		co.Path = u.Path
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		etag, err := strconv.Unquote(etag)
		if err != nil {
			return err
		}
		co.ETag = etag
	}
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		t, err := http.ParseTime(lastModified)
		if err != nil {
			return err
		}
		co.ModTime = t
	}

	return nil
}

func (c *Client) PutCalendarObject(path string, cal *ical.Calendar) (*CalendarObject, error) {
	// TODO: add support for If-None-Match and If-Match

	// TODO: some servers want a Content-Length header, so we can't stream the
	// request body here. See the Radicale issue:
	// https://github.com/Kozea/Radicale/issues/1016

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).EncodeCalendar(cal); err != nil {
		return nil, err
	}

	req, err := c.ic.NewRequest(http.MethodPut, path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ical.MIMEType)

	resp, err := c.ic.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	co := &CalendarObject{Path: path}
	if err := populateCalendarObject(co, resp); err != nil {
		return nil, err
	}
	return co, nil
}
