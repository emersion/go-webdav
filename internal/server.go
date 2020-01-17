package internal

import (
	"net/http"
	"fmt"
	"mime"
	"encoding/xml"
)

type HTTPError struct {
	Code int
	Err  error
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

type Backend interface {
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
	w.Header().Add("Allow", "OPTIONS, GET, HEAD, PROPFIND")
	w.Header().Add("DAV", "1, 3")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request) error {
	t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if t != "application/xml" && t != "text/xml" {
		return HTTPErrorf(http.StatusBadRequest, "webdav: expected application/xml PROPFIND request")
	}

	var propfind Propfind
	if err := xml.NewDecoder(r.Body).Decode(&propfind); err != nil {
		return &HTTPError{http.StatusBadRequest, err}
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

	w.Header().Add("Content-Type", "text/xml; charset=\"utf-8\"")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(xml.Header))
	return xml.NewEncoder(w).Encode(&ms)
}
