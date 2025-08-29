package webdav

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/emersion/go-webdav/internal"
)

// FileSystem is a WebDAV server backend.
type FileSystem interface {
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	Stat(ctx context.Context, name string) (*FileInfo, error)
	ReadDir(ctx context.Context, name string, recursive bool) ([]FileInfo, error)
	Create(ctx context.Context, name string, body io.ReadCloser, opts *CreateOptions) (fileInfo *FileInfo, created bool, err error)
	RemoveAll(ctx context.Context, name string, opts *RemoveAllOptions) error
	Mkdir(ctx context.Context, name string) error
	Copy(ctx context.Context, name, dest string, options *CopyOptions) (created bool, err error)
	Move(ctx context.Context, name, dest string, options *MoveOptions) (created bool, err error)
}

// Handler handles WebDAV HTTP requests. It can be used to create a WebDAV
// server.
type Handler struct {
	FileSystem FileSystem

	locks   map[string]*internal.Lock
	locksMu sync.Mutex
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.FileSystem == nil {
		http.Error(w, "webdav: no filesystem available", http.StatusInternalServerError)
		return
	}

	h.locksMu.Lock()
	if h.locks == nil {
		h.locks = make(map[string]*internal.Lock)
	}
	h.locksMu.Unlock()

	b := backend{FileSystem: h.FileSystem, locks: h.locks, locksMu: &h.locksMu}
	hh := internal.Handler{Backend: &b}
	hh.ServeHTTP(w, r)
}

// NewHTTPError creates a new error that is associated with an HTTP status code
// and optionally an error that lead to it. Backends can use this functions to
// return errors that convey some semantics (e.g. 404 not found, 403 access
// denied, etc.) while also providing an (optional) arbitrary error context
// (intended for humans).
func NewHTTPError(statusCode int, cause error) error {
	return &internal.HTTPError{Code: statusCode, Err: cause}
}

type backend struct {
	FileSystem FileSystem

	locks   map[string]*internal.Lock
	locksMu *sync.Mutex
}

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	caps = []string{"2"}

	fi, err := b.FileSystem.Stat(r.Context(), r.URL.Path)
	if internal.IsNotFound(err) {
		return caps, []string{http.MethodOptions, http.MethodPut, "MKCOL"}, nil
	} else if err != nil {
		return nil, nil, err
	}

	allow = []string{
		http.MethodOptions,
		http.MethodDelete,
		"PROPFIND",
		"COPY",
		"MOVE",
	}

	if !fi.IsDir {
		allow = append(allow, http.MethodHead, http.MethodGet, http.MethodPut)
	}

	return caps, allow, nil
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	fi, err := b.FileSystem.Stat(r.Context(), r.URL.Path)
	if err != nil {
		return err
	}
	if fi.IsDir {
		return &internal.HTTPError{Code: http.StatusMethodNotAllowed}
	}

	f, err := b.FileSystem.Open(r.Context(), r.URL.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))
	if fi.MIMEType != "" {
		w.Header().Set("Content-Type", fi.MIMEType)
	}
	if !fi.ModTime.IsZero() {
		w.Header().Set("Last-Modified", fi.ModTime.UTC().Format(http.TimeFormat))
	}
	if fi.ETag != "" {
		w.Header().Set("ETag", internal.ETag(fi.ETag).String())
	}

	if rs, ok := f.(io.ReadSeeker); ok {
		// If it's an io.Seeker, use http.ServeContent which supports ranges
		http.ServeContent(w, r, r.URL.Path, fi.ModTime, rs)
	} else {
		if r.Method != http.MethodHead {
			io.Copy(w, f)
		}
	}
	return nil
}

func (b *backend) PropFind(r *http.Request, propfind *internal.PropFind, depth internal.Depth) (*internal.MultiStatus, error) {
	// TODO: use partial error Response on error

	fi, err := b.FileSystem.Stat(r.Context(), r.URL.Path)
	if err != nil {
		return nil, err
	}

	var resps []internal.Response
	if depth != internal.DepthZero && fi.IsDir {
		children, err := b.FileSystem.ReadDir(r.Context(), r.URL.Path, depth == internal.DepthInfinity)
		if err != nil {
			return nil, err
		}

		resps = make([]internal.Response, len(children))
		for i, child := range children {
			resp, err := b.propFindFile(propfind, &child)
			if err != nil {
				return nil, err
			}
			resps[i] = *resp
		}
	} else {
		resp, err := b.propFindFile(propfind, fi)
		if err != nil {
			return nil, err
		}

		resps = []internal.Response{*resp}
	}

	return internal.NewMultiStatus(resps...), nil
}

func (b *backend) propFindFile(propfind *internal.PropFind, fi *FileInfo) (*internal.Response, error) {
	props := make(map[xml.Name]internal.PropFindFunc)

	props[internal.ResourceTypeName] = func(*internal.RawXMLValue) (interface{}, error) {
		var types []xml.Name
		if fi.IsDir {
			types = append(types, internal.CollectionName)
		}
		return internal.NewResourceType(types...), nil
	}

	props[internal.SupportedLockName] = internal.PropFindValue(&internal.SupportedLock{
		LockEntries: []internal.LockEntry{{
			LockScope: internal.LockScope{Exclusive: &struct{}{}},
			LockType:  internal.LockType{Write: &struct{}{}},
		}},
	})

	if !fi.IsDir {
		props[internal.GetContentLengthName] = internal.PropFindValue(&internal.GetContentLength{
			Length: fi.Size,
		})

		if !fi.ModTime.IsZero() {
			props[internal.GetLastModifiedName] = internal.PropFindValue(&internal.GetLastModified{
				LastModified: internal.Time(fi.ModTime),
			})
		}

		if fi.MIMEType != "" {
			props[internal.GetContentTypeName] = internal.PropFindValue(&internal.GetContentType{
				Type: fi.MIMEType,
			})
		}

		if fi.ETag != "" {
			props[internal.GetETagName] = internal.PropFindValue(&internal.GetETag{
				ETag: internal.ETag(fi.ETag),
			})
		}
	}

	return internal.NewPropFindResponse(fi.Path, propfind, props)
}

func (b *backend) PropPatch(r *http.Request, update *internal.PropertyUpdate) (*internal.Response, error) {
	fi, err := b.FileSystem.Stat(r.Context(), r.URL.Path)
	if err != nil {
		return nil, err
	}

	resp := &internal.Response{Hrefs: []internal.Href{internal.Href{Path: fi.Path}}}

	for _, set := range update.Set {
		for _, raw := range set.Prop.Raw {
			xmlName, ok := raw.XMLName()
			if !ok {
				continue
			}

			emptyVal := internal.NewRawXMLElement(xmlName, nil, nil)

			if err := resp.EncodeProp(http.StatusForbidden, emptyVal); err != nil {
				return nil, err
			}
		}

	}

	for _, remove := range update.Remove {
		for _, raw := range remove.Prop.Raw {
			xmlName, ok := raw.XMLName()
			if !ok {
				continue
			}

			emptyVal := internal.NewRawXMLElement(xmlName, nil, nil)

			if err := resp.EncodeProp(http.StatusForbidden, emptyVal); err != nil {
				return nil, err
			}
		}

	}

	if len(resp.PropStats) == 0 {
		return nil, internal.HTTPErrorf(http.StatusBadRequest,
			"webdav: request missing properties to update")
	}

	return resp, nil
}

func (b *backend) Put(w http.ResponseWriter, r *http.Request) error {
	if lock := b.resourceLock(r.URL.Path); lock != nil {
		token, err := internal.ParseSubmittedToken(r.Header)
		if err != nil {
			return err
		}
		if token != lock.Href {
			return &internal.HTTPError{Code: http.StatusLocked}
		}
	}

	ifNoneMatch := ConditionalMatch(r.Header.Get("If-None-Match"))
	ifMatch := ConditionalMatch(r.Header.Get("If-Match"))

	opts := CreateOptions{
		IfNoneMatch: ifNoneMatch,
		IfMatch:     ifMatch,
	}
	fi, created, err := b.FileSystem.Create(r.Context(), r.URL.Path, r.Body, &opts)
	if err != nil {
		return err
	}

	if fi.MIMEType != "" {
		w.Header().Set("Content-Type", fi.MIMEType)
	}
	if !fi.ModTime.IsZero() {
		w.Header().Set("Last-Modified", fi.ModTime.UTC().Format(http.TimeFormat))
	}
	if fi.ETag != "" {
		w.Header().Set("ETag", internal.ETag(fi.ETag).String())
	}

	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}

	return nil
}

func (b *backend) Delete(r *http.Request) error {
	if lock := b.resourceLock(r.URL.Path); lock != nil {
		token, err := internal.ParseSubmittedToken(r.Header)
		if err != nil {
			return err
		}
		if token != lock.Href {
			return &internal.HTTPError{Code: http.StatusLocked}
		}
	}

	ifNoneMatch := ConditionalMatch(r.Header.Get("If-None-Match"))
	ifMatch := ConditionalMatch(r.Header.Get("If-Match"))

	opts := RemoveAllOptions{
		IfNoneMatch: ifNoneMatch,
		IfMatch:     ifMatch,
	}
	err := b.FileSystem.RemoveAll(r.Context(), r.URL.Path, &opts)
	if err == nil {
		// URL became unmapped so delete lock if exists
		b.locksMu.Lock()
		defer b.locksMu.Unlock()
		delete(b.locks, r.URL.Path)
	}

	return err
}

func (b *backend) Mkcol(r *http.Request) error {
	if r.Header.Get("Content-Type") != "" {
		return internal.HTTPErrorf(http.StatusUnsupportedMediaType, "webdav: request body not supported in MKCOL request")
	}
	err := b.FileSystem.Mkdir(r.Context(), r.URL.Path)
	if internal.IsNotFound(err) {
		return &internal.HTTPError{Code: http.StatusConflict, Err: err}
	}
	return err
}

func (b *backend) Copy(r *http.Request, dest *internal.Href, recursive, overwrite bool) (created bool, err error) {
	if lock := b.resourceLock(dest.Path); lock != nil {
		token, err := internal.ParseSubmittedToken(r.Header)
		if err != nil {
			return false, err
		}
		if token != lock.Href {
			return false, &internal.HTTPError{Code: http.StatusLocked}
		}
	}

	options := CopyOptions{
		NoRecursive: !recursive,
		NoOverwrite: !overwrite,
	}
	created, err = b.FileSystem.Copy(r.Context(), r.URL.Path, dest.Path, &options)
	if os.IsExist(err) {
		return false, &internal.HTTPError{http.StatusPreconditionFailed, err}
	}
	return created, err
}

func (b *backend) Move(r *http.Request, dest *internal.Href, overwrite bool) (created bool, err error) {
	// Check source and destination locks
	var conditions [][]internal.Condition
	hif := r.Header.Get("If")
	if hif == "" {
		conditions = nil
	} else {
		var err error
		conditions, err = internal.ParseConditions(hif)
		if err != nil {
			return false, &internal.HTTPError{http.StatusBadRequest, err}
		}
	}
	srcLock := b.resourceLock(r.URL.Path)
	destLock := b.resourceLock(dest.Path)
	for _, conds := range conditions {
		if len(conds) == 0 {
			continue
		}
		if len(conds) > 1 {
			return false, internal.HTTPErrorf(http.StatusBadRequest, "webdav: multiple conditions are not supported in the If header field")
		}
		if (conds[0].Resource == "" || conds[0].Resource == r.URL.Path) && srcLock != nil && conds[0].Token == srcLock.Href {
			srcLock = nil
		} else if (conds[0].Resource == dest.Path) && destLock != nil && conds[0].Token == destLock.Href {
			destLock = nil
		}
	}
	if srcLock != nil || destLock != nil {
		return false, &internal.HTTPError{Code: http.StatusLocked}
	}

	options := MoveOptions{
		NoOverwrite: !overwrite,
	}
	created, err = b.FileSystem.Move(r.Context(), r.URL.Path, dest.Path, &options)
	if os.IsExist(err) {
		return false, &internal.HTTPError{http.StatusPreconditionFailed, err}
	}
	if err == nil {
		// URL became unmapped so delete lock if exists
		b.locksMu.Lock()
		defer b.locksMu.Unlock()
		delete(b.locks, r.URL.Path)
	}
	return created, err
}

func (b *backend) Lock(r *http.Request, depth internal.Depth, timeout time.Duration, refreshToken string) (lock *internal.Lock, created bool, err error) {
	// TODO: locking unmapped URLs
	fi, err := b.FileSystem.Stat(r.Context(), r.URL.Path)
	if err != nil {
		return nil, false, err
	}
	if fi.IsDir {
		return nil, false, internal.HTTPErrorf(http.StatusBadRequest, "webdav: locking collections is not supported")
	}

	if refreshToken != "" {
		if lock := b.resourceLock(r.URL.Path); lock == nil || lock.Href != refreshToken {
			return nil, false, &internal.HTTPError{Code: http.StatusPreconditionFailed}
		} else {
			// Lock timeout is not supported so refresh is a no-op
			return lock, false, nil
		}
	}

	token := "opaquelocktoken:" + uuid.NewString()
	lock = &internal.Lock{Href: token, Root: fi.Path, Timeout: 0 /* infinity */}

	b.locksMu.Lock()
	defer b.locksMu.Unlock()
	if _, prs := b.locks[r.URL.Path]; prs {
		return nil, false, internal.HTTPErrorf(http.StatusLocked, "webdav: there is already a lock on this resource")
	}
	b.locks[lock.Root] = lock

	return lock, false, nil
}

func (b *backend) Unlock(r *http.Request, tokenHref string) error {
	if lock := b.resourceLock(r.URL.Path); lock == nil {
		return internal.HTTPErrorf(http.StatusConflict, "webdav: resource is not locked")
	} else if lock.Href != tokenHref {
		return internal.HTTPErrorf(http.StatusForbidden, "webdav: incorrect token")
	}

	b.locksMu.Lock()
	defer b.locksMu.Unlock()
	delete(b.locks, r.URL.Path)

	return nil
}

func (b *backend) resourceLock(path string) *internal.Lock {
	b.locksMu.Lock()
	defer b.locksMu.Unlock()

	lock, prs := b.locks[path]
	if !prs {
		return nil
	}

	return lock
}

// BackendSuppliedHomeSet represents either a CalDAV calendar-home-set or a
// CardDAV addressbook-home-set. It should only be created via
// caldav.NewCalendarHomeSet or carddav.NewAddressBookHomeSet. Only to
// be used server-side, for listing a user's home sets as determined by the
// (external) backend.
type BackendSuppliedHomeSet interface {
	GetXMLName() xml.Name
}

// UserPrincipalBackend can determine the current user's principal URL for a
// given request context.
type UserPrincipalBackend interface {
	CurrentUserPrincipal(ctx context.Context) (string, error)
}

// Capability indicates the features that a server supports.
type Capability string

// ServePrincipalOptions holds options for ServePrincipal.
type ServePrincipalOptions struct {
	CurrentUserPrincipalPath string
	HomeSets                 []BackendSuppliedHomeSet
	Capabilities             []Capability
}

// ServePrincipal replies to requests for a principal URL.
func ServePrincipal(w http.ResponseWriter, r *http.Request, options *ServePrincipalOptions) {
	switch r.Method {
	case http.MethodOptions:
		caps := []string{"1", "3"}
		for _, c := range options.Capabilities {
			caps = append(caps, string(c))
		}
		allow := []string{http.MethodOptions, "PROPFIND", "REPORT", "DELETE", "MKCOL"}
		w.Header().Add("DAV", strings.Join(caps, ", "))
		w.Header().Add("Allow", strings.Join(allow, ", "))
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		if err := servePrincipalPropfind(w, r, options); err != nil {
			internal.ServeError(w, err)
		}
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func servePrincipalPropfind(w http.ResponseWriter, r *http.Request, options *ServePrincipalOptions) error {
	var propfind internal.PropFind
	if err := internal.DecodeXMLRequest(r, &propfind); err != nil {
		return err
	}
	props := map[xml.Name]internal.PropFindFunc{
		internal.ResourceTypeName: func(*internal.RawXMLValue) (interface{}, error) {
			return internal.NewResourceType(principalName), nil
		},
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (interface{}, error) {
			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: options.CurrentUserPrincipalPath}}, nil
		},
	}

	// TODO: handle Depth and more properties

	for _, homeSet := range options.HomeSets {
		hs := homeSet // capture variable for closure
		props[homeSet.GetXMLName()] = func(*internal.RawXMLValue) (interface{}, error) {
			return hs, nil
		}
	}

	resp, err := internal.NewPropFindResponse(r.URL.Path, &propfind, props)
	if err != nil {
		return err
	}

	ms := internal.NewMultiStatus(*resp)
	return internal.ServeMultiStatus(w, ms)
}
