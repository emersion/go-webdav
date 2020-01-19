package internal

import (
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"strings"
)

type HTTPError struct {
	Code int
	Err  error
}

func HTTPErrorFromError(err error) *HTTPError {
	if err == nil {
		return nil
	}
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr
	} else {
		return &HTTPError{http.StatusInternalServerError, err}
	}
}

func HTTPErrorf(code int, format string, a ...interface{}) *HTTPError {
	return &HTTPError{code, fmt.Errorf(format, a...)}
}

func (err *HTTPError) Error() string {
	s := fmt.Sprintf("%v %v", err.Code, http.StatusText(err.Code))
	if err.Err != nil {
		return fmt.Sprintf("%v: %v", s, err.Err)
	} else {
		return s
	}
}

func ServeError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if httpErr, ok := err.(*HTTPError); ok {
		code = httpErr.Code
	}
	http.Error(w, err.Error(), code)
}

func DecodeXMLRequest(r *http.Request, v interface{}) error {
	t, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if t != "application/xml" && t != "text/xml" {
		return HTTPErrorf(http.StatusBadRequest, "webdav: expected application/xml request")
	}

	if err := xml.NewDecoder(r.Body).Decode(v); err != nil {
		return &HTTPError{http.StatusBadRequest, err}
	}
	return nil
}

func ServeXML(w http.ResponseWriter) *xml.Encoder {
	w.Header().Add("Content-Type", "text/xml; charset=\"utf-8\"")
	w.Write([]byte(xml.Header))
	return xml.NewEncoder(w)
}

func ServeMultistatus(w http.ResponseWriter, ms *Multistatus) error {
	// TODO: streaming
	w.WriteHeader(http.StatusMultiStatus)
	return ServeXML(w).Encode(ms)
}

type Backend interface {
	Options(r *http.Request) ([]string, error)
	HeadGet(w http.ResponseWriter, r *http.Request) error
	Propfind(r *http.Request, pf *Propfind, depth Depth) (*Multistatus, error)
}

type Handler struct {
	Backend Backend
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	if h.Backend == nil {
		err = fmt.Errorf("webdav: no backend available")
	} else {
		switch r.Method {
		case http.MethodOptions:
			err = h.handleOptions(w, r)
		case http.MethodGet, http.MethodHead:
			err = h.Backend.HeadGet(w, r)
		case "PROPFIND":
			err = h.handlePropfind(w, r)
		default:
			err = HTTPErrorf(http.StatusMethodNotAllowed, "webdav: unsupported method")
		}
	}

	if err != nil {
		code := http.StatusInternalServerError
		if httpErr, ok := err.(*HTTPError); ok {
			code = httpErr.Code
		}
		http.Error(w, err.Error(), code)
	}
}

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) error {
	methods, err := h.Backend.Options(r)
	if err != nil {
		return err
	}

	w.Header().Add("Allow", strings.Join(methods, ", "))
	w.Header().Add("DAV", "1, 3")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request) error {
	var propfind Propfind
	if err := DecodeXMLRequest(r, &propfind); err != nil {
		return err
	}

	depth := DepthInfinity
	if s := r.Header.Get("Depth"); s != "" {
		var err error
		depth, err = ParseDepth(s)
		if err != nil {
			return &HTTPError{http.StatusBadRequest, err}
		}
	}

	ms, err := h.Backend.Propfind(r, &propfind, depth)
	if err != nil {
		return err
	}

	return ServeMultistatus(w, ms)
}

type PropfindFunc func(raw *RawXMLValue) (interface{}, error)

func NewPropfindResponse(href string, propfind *Propfind, props map[xml.Name]PropfindFunc) (*Response, error) {
	resp := NewOKResponse(href)

	if _, ok := props[ResourceTypeName]; !ok {
		props[ResourceTypeName] = func(*RawXMLValue) (interface{}, error) {
			return NewResourceType(), nil
		}
	}

	if propfind.PropName != nil {
		for xmlName, _ := range props {
			emptyVal := NewRawXMLElement(xmlName, nil, nil)
			if err := resp.EncodeProp(http.StatusOK, emptyVal); err != nil {
				return nil, err
			}
		}
	} else if propfind.AllProp != nil {
		// TODO: add support for propfind.Include
		for xmlName, f := range props {
			emptyVal := NewRawXMLElement(xmlName, nil, nil)

			val, err := f(emptyVal)

			code := http.StatusOK
			if err != nil {
				// TODO: don't throw away error message here
				code = HTTPErrorFromError(err).Code
				val = emptyVal
			}

			if err := resp.EncodeProp(code, val); err != nil {
				return nil, err
			}
		}
	} else if prop := propfind.Prop; prop != nil {
		for _, raw := range prop.Raw {
			xmlName, ok := raw.XMLName()
			if !ok {
				continue
			}

			emptyVal := NewRawXMLElement(xmlName, nil, nil)

			var code int
			var val interface{} = emptyVal
			f, ok := props[xmlName]
			if ok {
				if v, err := f(&raw); err != nil {
					// TODO: don't throw away error message here
					code = HTTPErrorFromError(err).Code
				} else {
					code = http.StatusOK
					val = v
				}
			} else {
				code = http.StatusNotFound
			}

			if err := resp.EncodeProp(code, val); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, HTTPErrorf(http.StatusBadRequest, "webdav: request missing propname, allprop or prop element")
	}

	return resp, nil
}
