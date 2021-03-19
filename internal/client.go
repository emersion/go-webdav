package internal

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"
)

// Discover performs a DNS-based CalDAV/CardDAV service discovery as described
// in RFC 6764 section 6. It returns the URL to the CalDAV/CardDAV server.
func Discover(service string, host string) (string, error) {
	if service != "caldav" && service != "carddav" {
		return "", fmt.Errorf("webdav: service discovery of type %v not supported", service)
	}

	path := ""

	// Check for SRV records for the service we want, only lookup secure versions
	// (caldavs, carddavs), plaintext connections are insecure
	_, addrs, err := net.LookupSRV(fmt.Sprintf("%vs", service), "tcp", host)
	if dnsErr, ok := err.(*net.DNSError); ok {
		if dnsErr.IsTemporary {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	if len(addrs) > 0 {
		srvTarget := strings.TrimSuffix(addrs[0].Target, ".")

		// If we found one, check for a TXT record specifying the path
		if srvTarget != "" {
			txtRecs, err := net.LookupTXT(fmt.Sprintf("_%vs._tcp.%v", service, host))
			if dnsErr, ok := err.(*net.DNSError); ok {
				if dnsErr.IsTemporary {
					return "", err
				}
			} else if err != nil {
				return "", err
			}

			for _, txtRec := range txtRecs {
				// This is not correct according to RFC 6763 sections 6.3 to 6.5,
				// but LookupTXT merges all constituent strings together
				for _, txtRecKeyVal := range strings.Split(txtRec, " ") {
					if strings.HasPrefix(strings.ToLower(txtRecKeyVal), "path=") {
						path = txtRecKeyVal[5:]
						break
					}
				}

				if path != "" {
					break
				}
			}

			if addrs[0].Port == 443 {
				host = srvTarget
			} else {
				host = fmt.Sprintf("%v:%v", srvTarget, addrs[0].Port)
			}
		}
	}

	// If we didn't get a path from TXT records, use the default well-known location
	if path == "" {
		path = fmt.Sprintf("/.well-known/%v", service)
	}

	u := url.URL{Scheme: "https", Host: host, Path: path}
	serviceUrl := u.String()

	// Check if the resulting URL hosts a service
	req, err := http.NewRequest(http.MethodOptions, serviceUrl, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	// Servers might require authentication to perform an OPTIONS request
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("HTTP request to %v failed: %v %v", serviceUrl, resp.StatusCode, resp.Status)
	}

	return serviceUrl, nil
}

// HTTPClient performs HTTP requests. It's implemented by *http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	http     HTTPClient
	endpoint *url.URL
}

func NewClient(c HTTPClient, endpoint string) (*Client, error) {
	if c == nil {
		c = http.DefaultClient
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if u.Path == "" {
		// This is important to avoid issues with path.Join
		u.Path = "/"
	}
	return &Client{http: c, endpoint: u}, nil
}

func (c *Client) ResolveHref(p string) *url.URL {
	if !strings.HasPrefix(p, "/") {
		p = path.Join(c.endpoint.Path, p)
	}
	return &url.URL{
		Scheme: c.endpoint.Scheme,
		User:   c.endpoint.User,
		Host:   c.endpoint.Host,
		Path:   p,
	}
}

func (c *Client) NewRequest(method string, path string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, c.ResolveHref(path).String(), body)
}

func (c *Client) NewXMLRequest(method string, path string, v interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	if err := xml.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}

	req, err := c.NewRequest(method, path, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")

	return req, nil
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/plain"
		}

		var wrappedErr error
		t, _, _ := mime.ParseMediaType(contentType)
		if t == "application/xml" || t == "text/xml" {
			var davErr Error
			if err := xml.NewDecoder(resp.Body).Decode(&davErr); err != nil {
				wrappedErr = err
			} else {
				wrappedErr = &davErr
			}
		} else if strings.HasPrefix(t, "text/") {
			lr := io.LimitedReader{R: resp.Body, N: 1024}
			var buf bytes.Buffer
			io.Copy(&buf, &lr)
			resp.Body.Close()
			if s := strings.TrimSpace(buf.String()); s != "" {
				if lr.N == 0 {
					s += " […]"
				}
				wrappedErr = fmt.Errorf("%v", s)
			}
		}
		return nil, &HTTPError{Code: resp.StatusCode, Err: wrappedErr}
	}
	return resp, nil
}

func (c *Client) DoMultiStatus(req *http.Request) (*Multistatus, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("HTTP multi-status request failed: %v", resp.Status)
	}

	// TODO: the response can be quite large, support streaming Response elements
	var ms Multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}

	return &ms, nil
}

func (c *Client) Propfind(path string, depth Depth, propfind *Propfind) (*Multistatus, error) {
	req, err := c.NewXMLRequest("PROPFIND", path, propfind)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", depth.String())

	return c.DoMultiStatus(req)
}

// PropfindFlat performs a PROPFIND request with a zero depth.
func (c *Client) PropfindFlat(path string, propfind *Propfind) (*Response, error) {
	ms, err := c.Propfind(path, DepthZero, propfind)
	if err != nil {
		return nil, err
	}

	return ms.Get(c.ResolveHref(path).Path)
}

func parseCommaSeparatedSet(values []string, upper bool) map[string]bool {
	m := make(map[string]bool)
	for _, v := range values {
		fields := strings.FieldsFunc(v, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		for _, f := range fields {
			if upper {
				f = strings.ToUpper(f)
			} else {
				f = strings.ToLower(f)
			}
			m[f] = true
		}
	}
	return m
}

func (c *Client) Options(path string) (classes map[string]bool, methods map[string]bool, err error) {
	req, err := c.NewRequest(http.MethodOptions, path, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}
	resp.Body.Close()

	classes = parseCommaSeparatedSet(resp.Header["Dav"], false)
	if !classes["1"] {
		return nil, nil, fmt.Errorf("webdav: server doesn't support DAV class 1")
	}

	methods = parseCommaSeparatedSet(resp.Header["Allow"], true)
	return classes, methods, nil
}

// SyncCollection perform a `sync-collection` REPORT operation on a resource
func (c *Client) SyncCollection(path, syncToken string, level Depth, limit *Limit, prop *Prop) (*Multistatus, error) {
	q := SyncCollectionQuery{
		SyncToken: syncToken,
		SyncLevel: level.String(),
		Limit:     limit,
		Prop:      prop,
	}

	req, err := c.NewXMLRequest("REPORT", path, &q)
	if err != nil {
		return nil, err
	}

	ms, err := c.DoMultiStatus(req)
	if err != nil {
		return nil, err
	}

	return ms, nil
}
