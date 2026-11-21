package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"mime"
	"net/http"
	"net/url"

	"github.com/emersion/go-ical"

	"github.com/emersion/go-webdav/internal"
)

// ScheduleBackend is a Backend that answers free/busy requests POSTed to
// the principal's scheduling outbox (RFC 6638 §5, the VFREEBUSY part
// alone). Implementing it makes the principal carry schedule-outbox-URL
// and calendar-user-address-set; it does not make the server schedule
// (calendar-auto-schedule stays unannounced, so clients keep sending
// their invitations themselves).
type ScheduleBackend interface {
	// ScheduleOutboxPath is the caller's outbox.
	ScheduleOutboxPath(ctx context.Context) (string, error)
	// CalendarUserAddresses are the caller's calendar user addresses
	// (mailto: URIs).
	CalendarUserAddresses(ctx context.Context) ([]string, error)
	// FreeBusy answers a VFREEBUSY request (METHOD:REQUEST), one
	// response per recipient.
	FreeBusy(ctx context.Context, request *ical.Calendar) ([]ScheduleResponse, error)
}

// ScheduleResponse is one recipient's answer: RequestStatus as RFC 5545
// §3.8.8.3 puts it ("2.0;Success", "3.7;Invalid calendar user") and, on
// success, the VFREEBUSY reply.
type ScheduleResponse struct {
	Recipient     string
	RequestStatus string
	Calendar      *ical.Calendar
}

var (
	scheduleOutboxURLName      = xml.Name{Space: namespace, Local: "schedule-outbox-URL"}
	calendarUserAddressSetName = xml.Name{Space: namespace, Local: "calendar-user-address-set"}
	scheduleOutboxName         = xml.Name{Space: namespace, Local: "schedule-outbox"}
)

type scheduleOutboxURL struct {
	XMLName xml.Name      `xml:"urn:ietf:params:xml:ns:caldav schedule-outbox-URL"`
	Href    internal.Href `xml:"DAV: href"`
}

type calendarUserAddressSet struct {
	XMLName xml.Name        `xml:"urn:ietf:params:xml:ns:caldav calendar-user-address-set"`
	Hrefs   []internal.Href `xml:"DAV: href"`
}

type scheduleResponse struct {
	XMLName   xml.Name                 `xml:"urn:ietf:params:xml:ns:caldav schedule-response"`
	Responses []scheduleRecipientReply `xml:"urn:ietf:params:xml:ns:caldav response"`
}

type scheduleRecipient struct {
	Href internal.Href `xml:"DAV: href"`
}

type scheduleRecipientReply struct {
	Recipient     scheduleRecipient `xml:"urn:ietf:params:xml:ns:caldav recipient"`
	RequestStatus string            `xml:"urn:ietf:params:xml:ns:caldav request-status"`
	CalendarData  string            `xml:"urn:ietf:params:xml:ns:caldav calendar-data,omitempty"`
}

// uriHref is a calendar user address (a mailto: URI) as an href; Href's
// Path would print it as a relative path.
func uriHref(uri string) internal.Href {
	u, err := url.Parse(uri)
	if err != nil {
		return internal.Href{Path: uri}
	}
	return internal.Href(*u)
}

// scheduleProps are the principal's scheduling properties, when the
// backend schedules.
func scheduleProps(ctx context.Context, b Backend) (map[xml.Name]internal.PropFindFunc, error) {
	sb, ok := b.(ScheduleBackend)
	if !ok {
		return nil, nil
	}
	outbox, err := sb.ScheduleOutboxPath(ctx)
	if err != nil {
		return nil, err
	}
	addrs, err := sb.CalendarUserAddresses(ctx)
	if err != nil {
		return nil, err
	}
	set := &calendarUserAddressSet{}
	for _, a := range addrs {
		set.Hrefs = append(set.Hrefs, uriHref(a))
	}
	return map[xml.Name]internal.PropFindFunc{
		scheduleOutboxURLName:      internal.PropFindValue(&scheduleOutboxURL{Href: internal.Href{Path: outbox}}),
		calendarUserAddressSetName: internal.PropFindValue(set),
	}, nil
}

// serveOutbox answers a request to the outbox; false when r is not one.
func (h *Handler) serveOutbox(w http.ResponseWriter, r *http.Request) (bool, error) {
	sb, ok := h.Backend.(ScheduleBackend)
	if !ok {
		return false, nil
	}
	outbox, err := sb.ScheduleOutboxPath(r.Context())
	if err != nil || r.URL.Path != outbox {
		return false, err
	}
	switch r.Method {
	case http.MethodOptions:
		w.Header().Add("DAV", "1, 3, calendar-access")
		w.Header().Add("Allow", "OPTIONS, PROPFIND, POST")
		w.WriteHeader(http.StatusNoContent)
		return true, nil
	case "PROPFIND":
		var propfind internal.PropFind
		if err := internal.DecodeXMLRequest(r, &propfind); err != nil {
			return true, err
		}
		resp, err := internal.NewPropFindResponse(outbox, &propfind, map[xml.Name]internal.PropFindFunc{
			internal.ResourceTypeName: internal.PropFindValue(internal.NewResourceType(internal.CollectionName, scheduleOutboxName)),
		})
		if err != nil {
			return true, err
		}
		return true, internal.ServeMultiStatus(w, internal.NewMultiStatus(*resp))
	case http.MethodPost:
		t, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if t != ical.MIMEType {
			return true, internal.HTTPErrorf(http.StatusUnsupportedMediaType, "caldav: expected %s", ical.MIMEType)
		}
		cal, err := ical.NewDecoder(r.Body).Decode()
		if err != nil {
			return true, internal.HTTPErrorf(http.StatusBadRequest, "caldav: %v", err)
		}
		if p := cal.Props.Get(ical.PropMethod); p == nil || p.Value != "REQUEST" || len(cal.Children) == 0 || cal.Children[0].Name != ical.CompFreeBusy {
			return true, internal.HTTPErrorf(http.StatusForbidden, "caldav: the outbox answers VFREEBUSY requests only")
		}
		replies, err := sb.FreeBusy(r.Context(), cal)
		if err != nil {
			return true, err
		}
		out := scheduleResponse{}
		for _, rep := range replies {
			x := scheduleRecipientReply{Recipient: scheduleRecipient{uriHref(rep.Recipient)}, RequestStatus: rep.RequestStatus}
			if rep.Calendar != nil {
				var buf bytes.Buffer
				if err := ical.NewEncoder(&buf).Encode(rep.Calendar); err != nil {
					return true, err
				}
				x.CalendarData = buf.String()
			}
			out.Responses = append(out.Responses, x)
		}
		w.WriteHeader(http.StatusOK)
		return true, internal.ServeXML(w).Encode(&out)
	}
	return true, internal.HTTPErrorf(http.StatusMethodNotAllowed, "caldav: the outbox takes POST")
}
