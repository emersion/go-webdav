// Package carddav provides a CardDAV server implementation, as defined in
// RFC 6352.
package carddav

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/url"
	"os"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"golang.org/x/net/context"

	"log"
)

var addressDataName = xml.Name{Space: "urn:ietf:params:xml:ns:carddav", Local: "address-data"}

type responseWriter struct {
	http.ResponseWriter
}

func (w responseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

type Handler struct {
	ab     AddressBook
	webdav *webdav.Handler
}

func NewHandler(ab AddressBook) *Handler {
	return &Handler{
		ab: ab,
		webdav: &webdav.Handler{
			FileSystem: &fileSystem{ab},
			Logger: func(req *http.Request, err error) {
				if err != nil {
					log.Println("ERROR", req, err)
				}
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Add("DAV", "addressbook")
	}

	w = responseWriter{w}
	switch r.Method {
	case "REPORT":
		code, err := h.handleReport(w, r)
		if err != nil {
			if code == 0 {
				code = http.StatusInternalServerError
			}
			http.Error(w, err.Error(), code)
		}
	case "OPTIONS":
		w.Header().Add("Allow", "REPORT")
		fallthrough
	default:
		h.webdav.ServeHTTP(w, r)
	}
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) (int, error) {
	var mg addressbookMultiget
	if err := xml.NewDecoder(r.Body).Decode(&mg); err != nil {
		return http.StatusBadRequest, err
	}

	mw := webdav.NewMultistatusWriter(w)
	defer mw.Close()

	if len(mg.Href) == 0 {
		mg.Href = []string{r.URL.Path}
	}
	for _, href := range mg.Href {
		pstats, status, err := multiget(r.Context(), h.webdav.FileSystem, h.webdav.LockSystem, href, []xml.Name(mg.Prop), mg.Allprop != nil)
		if err != nil {
			return status, err
		}

		resp := &webdav.Response{
			Href:     []string{(&url.URL{Path: href}).EscapedPath()},
			Status:   status,
			Propstat: pstats,
		}

		if err := mw.Write(resp); err != nil {
			return 0, err
		}
	}

	return 0, nil
}

func multiget(ctx context.Context, fs webdav.FileSystem, ls webdav.LockSystem, name string, pnames []xml.Name, allprop bool) ([]webdav.Propstat, int, error) {
	wantAddressData := false
	for i, pname := range pnames {
		if pname == addressDataName {
			pnames = append(pnames[:i], pnames[i+1:]...)
			wantAddressData = true
			break
		}
	}

	var pstats []webdav.Propstat
	var err error
	if allprop {
		wantAddressData = true
		pstats, err = webdav.Allprop(ctx, fs, ls, name, pnames)
	} else {
		pstats, err = webdav.Props(ctx, fs, ls, name, pnames)
	}
	if err != nil {
		return pstats, 0, err
	}

	// TODO: locking stuff

	f, err := fs.OpenFile(ctx, name, os.O_RDONLY, 0)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, http.StatusNotFound, err
	}

	if wantAddressData {
		if fi.IsDir() {
			// TODO
			return nil, http.StatusNotFound, err
		}

		prop, status, err := addressdata(f.(*file).ao)
		if err != nil {
			return nil, status, err
		}
		if status == 0 {
			status = http.StatusOK
		}

		inserted := false
		for i, pstat := range pstats {
			if pstat.Status == status {
				pstats[i].Props = append(pstat.Props, prop)
				inserted = true
				break
			}
		}

		if !inserted {
			pstats = append(pstats, webdav.Propstat{
				Props:  []webdav.Property{prop},
				Status: status,
			})
		}
	}

	return pstats, 0, nil
}

func addressdata(ao AddressObject) (webdav.Property, int, error) {
	prop := webdav.Property{XMLName: addressDataName}

	card, err := ao.Card()
	if err != nil {
		return prop, 0, err
	}

	// TODO: restrict to requested props

	var b bytes.Buffer
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		return prop, 0, err
	}

	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, b.Bytes()); err != nil {
		return prop, 0, err
	}

	prop.InnerXML = escaped.Bytes()
	return prop, 0, nil
}
