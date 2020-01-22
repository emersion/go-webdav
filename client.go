package webdav

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emersion/go-webdav/internal"
)

// Client provides access to a remote WebDAV filesystem.
type Client struct {
	ic *internal.Client
}

func NewClient(c *http.Client, endpoint string) (*Client, error) {
	ic, err := internal.NewClient(c, endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{ic}, nil
}

func (c *Client) SetBasicAuth(username, password string) {
	c.ic.SetBasicAuth(username, password)
}

func (c *Client) FindCurrentUserPrincipal() (string, error) {
	propfind := internal.NewPropNamePropfind(internal.CurrentUserPrincipalName)

	resp, err := c.ic.PropfindFlat("/", propfind)
	if err != nil {
		return "", err
	}

	var prop internal.CurrentUserPrincipal
	if err := resp.DecodeProp(&prop); err != nil {
		return "", err
	}
	if prop.Unauthenticated != nil {
		return "", fmt.Errorf("webdav: unauthenticated")
	}

	return prop.Href, nil
}

func fileInfoFromResponse(resp *internal.Response) (*FileInfo, error) {
	href, err := resp.Href()
	if err != nil {
		return nil, err
	}

	fi := &FileInfo{Href: href}

	var resType internal.ResourceType
	if err := resp.DecodeProp(&resType); err != nil {
		return nil, err
	}
	if resType.Is(internal.CollectionName) {
		fi.IsDir = true
	} else {
		var getLen internal.GetContentLength
		if err := resp.DecodeProp(&getLen); err != nil {
			return nil, err
		}

		var getMod internal.GetLastModified
		if err := resp.DecodeProp(&getMod); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		var getType internal.GetContentType
		if err := resp.DecodeProp(&getType); err != nil && !internal.IsNotFound(err) {
			return nil, err
		}

		fi.Size = getLen.Length
		fi.ModTime = time.Time(getMod.LastModified)
		fi.MIMEType = getType.Type
	}

	return fi, nil
}

// TODO: getetag
var fileInfoPropfind = internal.NewPropNamePropfind(
	internal.ResourceTypeName,
	internal.GetContentLengthName,
	internal.GetLastModifiedName,
	internal.GetContentTypeName,
)

func (c *Client) Stat(name string) (*FileInfo, error) {
	resp, err := c.ic.PropfindFlat(name, fileInfoPropfind)
	if err != nil {
		return nil, err
	}
	return fileInfoFromResponse(resp)
}

func (c *Client) Open(name string) (io.ReadCloser, error) {
	req, err := c.ic.NewRequest(http.MethodGet, name, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.ic.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (c *Client) Readdir(name string) ([]FileInfo, error) {
	// TODO: filter out the directory we're listing

	ms, err := c.ic.Propfind(name, internal.DepthOne, fileInfoPropfind)
	if err != nil {
		return nil, err
	}

	l := make([]FileInfo, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		fi, err := fileInfoFromResponse(&resp)
		if err != nil {
			return l, err
		}
		l = append(l, *fi)
	}

	return l, nil
}

type fileWriter struct {
	pw   *io.PipeWriter
	done <-chan error
}

func (fw *fileWriter) Write(b []byte) (int, error) {
	return fw.pw.Write(b)
}

func (fw *fileWriter) Close() error {
	if err := fw.pw.Close(); err != nil {
		return err
	}
	return <-fw.done
}

func (c *Client) Create(name string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()

	req, err := c.ic.NewRequest(http.MethodPut, name, pr)
	if err != nil {
		pw.Close()
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.ic.Do(req)
		done <- err
	}()

	return &fileWriter{pw, done}, nil
}

func (c *Client) RemoveAll(name string) error {
	req, err := c.ic.NewRequest(http.MethodDelete, name, nil)
	if err != nil {
		return err
	}

	_, err = c.ic.Do(req)
	return err
}

func (c *Client) Mkdir(name string) error {
	req, err := c.ic.NewRequest("MKCOL", name, nil)
	if err != nil {
		return err
	}

	_, err = c.ic.Do(req)
	return err
}

func (c *Client) MoveAll(name, dest string, overwrite bool) error {
	req, err := c.ic.NewRequest("MOVE", name, nil)
	if err != nil {
		return err
	}

	dest, err = c.ic.ResolveHref(dest)
	if err != nil {
		return err
	}
	req.Header.Set("Destination", dest)

	req.Header.Set("Overwrite", internal.FormatOverwrite(overwrite))

	_, err = c.ic.Do(req)
	return err
}
