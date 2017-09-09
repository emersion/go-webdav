package carddav

import (
	"bytes"
	"encoding/xml"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"golang.org/x/net/context"

	"log"
)

var (
	errNotYetImplemented = errors.New("not yet implemented")
	errUnsupported = errors.New("unsupported")
)

const nsDAV = "DAV:"

var (
	resourcetype = xml.Name{Space: nsDAV, Local: "resourcetype"}
	displayname = xml.Name{Space: nsDAV, Local: "displayname"}
	getcontenttype = xml.Name{Space: nsDAV, Local: "getcontenttype"}
)

const nsCardDAV = "urn:ietf:params:xml:ns:carddav"

var (
	addressBookDescription = xml.Name{Space: nsCardDAV, Local: "addressbook-description"}
	addressBookSupportedAddressData = xml.Name{Space: nsCardDAV, Local: "supported-address-data"}
	addressBookMaxResourceSize = xml.Name{Space: nsCardDAV, Local: "max-resource-size"}
	addressBookHomeSet = xml.Name{Space: nsCardDAV, Local: "addressbook-home-set"}
)

type fileInfo struct {
	name string
	size int64
	mode os.FileMode
	modTime time.Time
}

func (fi *fileInfo) Name() string {
	return fi.name
}

func (fi *fileInfo) Size() int64 {
	return fi.size
}

func (fi *fileInfo) Mode() os.FileMode {
	return fi.mode
}

func (fi *fileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *fileInfo) IsDir() bool {
	return fi.mode.IsDir()
}

func (fi *fileInfo) Sys() interface{} {
	return nil
}

type file struct {
	*bytes.Reader
	fs *fileSystem
	name string
	ao AddressObject
}

func (f *file) Close() error {
	return nil
}

func (f *file) Read(b []byte) (int, error) {
	if f.Reader == nil {
		card, err := f.ao.Card()
		if err != nil {
			return 0, err
		}

		var b bytes.Buffer
		if err := vcard.NewEncoder(&b).Encode(card); err != nil {
			return 0, err
		}

		f.Reader = bytes.NewReader(b.Bytes())
	}

	return f.Reader.Read(b)
}

func (f *file) Write(b []byte) (int, error) {
	return 0, errUnsupported
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if f.Reader == nil {
		if _, err := f.Read(nil); err != nil {
			return 0, err
		}
	}

	return f.Reader.Seek(offset, whence)
}

func (f *file) Readdir(count int) ([]os.FileInfo, error) {
	return nil, errUnsupported
}

func (f *file) Stat() (os.FileInfo, error) {
	info, err := f.ao.Stat()
	if info != nil || err != nil {
		return info, err
	}

	return &fileInfo{
		name: f.name,
		mode: os.ModePerm,
	}, nil
}

// TODO: getcontenttype for file

type dir struct {
	fs *fileSystem
	name string
	files []os.FileInfo

	n int
}

func (d *dir) Close() error {
	return nil
}

func (d *dir) Read(b []byte) (int, error) {
	return 0, errUnsupported
}

func (d *dir) Write(b []byte) (int, error) {
	return 0, errUnsupported
}

func (d *dir) Seek(offset int64, whence int) (int64, error) {
	return 0, errUnsupported
}

func (d *dir) Readdir(count int) ([]os.FileInfo, error) {
	if d.files == nil {
		aos, err := d.fs.ab.ListAddressObjects()
		if err != nil {
			return nil, err
		}

		d.files = make([]os.FileInfo, len(aos))
		for i, ao := range aos {
			f := &file{
				fs: d.fs,
				name: ao.ID() + ".vcf",
				ao: ao,
			}

			info, err := f.Stat()
			if err != nil {
				return nil, err
			}

			d.files[i] = info
		}
	}

	if count == 0 {
		count = len(d.files) - d.n
	}
	if d.n >= len(d.files) {
		return nil, nil
	}

	from := d.n
	d.n += count
	if d.n > len(d.files) {
		d.n = len(d.files)
	}

	return d.files[from:d.n], nil
}

func (d *dir) Stat() (os.FileInfo, error) {
	return &fileInfo{
		name: d.name,
		mode: os.ModeDir | os.ModePerm,
	}, nil
}

func (d *dir) DeadProps() (map[xml.Name]webdav.Property, error) {
	info, err := d.fs.ab.Info()
	if err != nil {
		return nil, err
	}

	return map[xml.Name]webdav.Property{
		resourcetype: webdav.Property{
			XMLName: resourcetype,
			InnerXML: []byte(`<collection xmlns="DAV:"/><addressbook xmlns="urn:ietf:params:xml:ns:carddav"/>`),
		},
		displayname: webdav.Property{
			XMLName: displayname,
			InnerXML: []byte(info.Name),
		},
		addressBookDescription: webdav.Property{
			XMLName: addressBookDescription,
			InnerXML: []byte(info.Description),
		},
		addressBookSupportedAddressData: webdav.Property{
			XMLName: addressBookSupportedAddressData,
			InnerXML: []byte(`<address-data-type xmlns="urn:ietf:params:xml:ns:carddav" content-type="text/vcard" version="3.0"/>` +
				`<address-data-type xmlns="urn:ietf:params:xml:ns:carddav" content-type="text/vcard" version="4.0"/>`),
		},
		addressBookMaxResourceSize: webdav.Property{
			XMLName: addressBookMaxResourceSize,
			InnerXML: []byte(strconv.Itoa(info.MaxResourceSize)),
		},
		addressBookHomeSet: webdav.Property{
			XMLName: addressBookHomeSet,
			InnerXML: []byte(`<href xmlns="DAV:">/</href>`),
		},
	}, nil
}

func (d *dir) Patch([]webdav.Proppatch) ([]webdav.Propstat, error) {
	return nil, errUnsupported
}

type fileSystem struct {
	ab AddressBook
}

func (fs *fileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return errNotYetImplemented
}

func (fs *fileSystem) addressObjectID(name string) string {
	return strings.TrimRight(strings.TrimLeft(name, "/"), ".vcf")
}

func (fs *fileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if name == "/" {
		return &dir{
			fs: fs,
			name: name,
		}, nil
	}

	id := fs.addressObjectID(name)
	ao, err := fs.ab.GetAddressObject(id)
	if err != nil {
		return nil, err
	}

	return &file{
		fs: fs,
		name: name,
		ao: ao,
	}, nil
}

func (fs *fileSystem) RemoveAll(ctx context.Context, name string) error {
	return errNotYetImplemented
}

func (fs *fileSystem) Rename(ctx context.Context, oldName, newName string) error {
	return errNotYetImplemented
}

func (fs *fileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if name == "/" {
		return &fileInfo{
			name: name,
			mode: os.ModeDir | os.ModePerm,
		}, nil
	}

	id := fs.addressObjectID(name)
	ao, err := fs.ab.GetAddressObject(id)
	if err != nil {
		return nil, err
	}

	info, err := ao.Stat()
	if info != nil || err != nil {
		return info, err
	}

	return &fileInfo{
		name: name,
		mode: os.ModePerm,
	}, nil
}

type Handler struct {
	webdav *webdav.Handler
}

func NewHandler(ab AddressBook) *Handler {
	return &Handler{&webdav.Handler{
		FileSystem: &fileSystem{ab},
		Logger: func(req *http.Request, err error) {
			if err != nil {
				log.Println("ERROR", req, err)
			}
		},
	}}
}

type responseWriter struct {
	http.ResponseWriter
}

func (w responseWriter) Write(b []byte) (int, error) {
	os.Stdout.Write(b)
	return w.ResponseWriter.Write(b)
}

func (h *Handler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Printf("%+v\n", req)
	if req.Method == http.MethodOptions {
		resp.Header().Add("DAV", "addressbook")
	}
	//h.webdav.ServeHTTP(resp, req)
	h.webdav.ServeHTTP(responseWriter{resp}, req)
}
