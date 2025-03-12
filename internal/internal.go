// Package internal provides low-level helpers for WebDAV clients and servers.
package internal

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Depth indicates whether a request applies to the resource's members. It's
// defined in RFC 4918 section 10.2.
type Depth int

const (
	// DepthZero indicates that the request applies only to the resource.
	DepthZero Depth = 0
	// DepthOne indicates that the request applies to the resource and its
	// internal members only.
	DepthOne Depth = 1
	// DepthInfinity indicates that the request applies to the resource and all
	// of its members.
	DepthInfinity Depth = -1
)

// ParseDepth parses a Depth header.
func ParseDepth(s string) (Depth, error) {
	switch s {
	case "0":
		return DepthZero, nil
	case "1":
		return DepthOne, nil
	case "infinity":
		return DepthInfinity, nil
	}
	return 0, fmt.Errorf("webdav: invalid Depth value")
}

// String formats the depth.
func (d Depth) String() string {
	switch d {
	case DepthZero:
		return "0"
	case DepthOne:
		return "1"
	case DepthInfinity:
		return "infinity"
	}
	panic("webdav: invalid Depth value")
}

// ParseOverwrite parses an Overwrite header.
func ParseOverwrite(s string) (bool, error) {
	switch s {
	case "T":
		return true, nil
	case "F":
		return false, nil
	}
	return false, fmt.Errorf("webdav: invalid Overwrite value")
}

// FormatOverwrite formats an Overwrite header.
func FormatOverwrite(overwrite bool) string {
	if overwrite {
		return "T"
	} else {
		return "F"
	}
}

type Timeout struct {
	Duration time.Duration
}

func ParseTimeout(s string) (Timeout, error) {
	if s == "Infinite" {
		return Timeout{}, nil
	} else if strings.HasPrefix(s, "Second-") {
		n, err := strconv.Atoi(strings.TrimPrefix(s, "Second-"))
		if err != nil || n <= 0 {
			return Timeout{}, fmt.Errorf("webdav: invalid Timeout value")
		}
		return Timeout{Duration: time.Duration(n) * time.Second}, nil
	} else {
		return Timeout{}, fmt.Errorf("webdav: invalid Timeout value")
	}
}

func (t Timeout) String() string {
	if t.Duration == 0 {
		return "Infinite"
	}
	return fmt.Sprintf("Second-%d", t.Duration/time.Second)
}

func ParseLockToken(s string) (string, error) {
	if !strings.HasPrefix(s, "<") || !strings.HasSuffix(s, ">") {
		return "", fmt.Errorf("webdav: invalid Lock-Token value")
	}
	return s[1 : len(s)-1], nil
}

func FormatLockToken(token string) string {
	return fmt.Sprintf("<%v>", token)
}

// Condition is a condition to match lock tokens and entity tags.
//
// Only one of Token or ETag is set.
type Condition struct {
	Resource string
	Not      bool
	Token    string
	ETag     string
}

type conditionParser struct {
	s string
}

func (p *conditionParser) acceptByte(ch byte) bool {
	if len(p.s) == 0 || p.s[0] != ch {
		return false
	}
	p.s = p.s[1:]
	return true
}

func (p *conditionParser) expectByte(ch byte) error {
	if len(p.s) == 0 {
		return fmt.Errorf("webdav: invalid If value: expected %q, got EOF", ch)
	} else if p.s[0] != ch {
		return fmt.Errorf("webdav: invalid If value: expected %q, got %q", ch, p.s[0])
	}
	p.s = p.s[1:]
	return nil
}

func (p *conditionParser) lws() bool {
	lws := false
	for p.acceptByte(' ') || p.acceptByte('\t') {
		lws = true
	}
	return lws
}

func (p *conditionParser) consumeUntilByte(ch byte) (string, error) {
	i := strings.IndexByte(p.s, ch)
	if i < 0 {
		return "", fmt.Errorf("webdav: invalid If value: expected %q, got EOF", ch)
	}
	s := p.s[:i]
	p.s = p.s[i+1:]
	return s, nil
}

func (p *conditionParser) condition() (*Condition, error) {
	not := strings.HasPrefix(p.s, "Not")
	if not {
		p.s = strings.TrimPrefix(p.s, "Not")
		p.lws()
	}

	if p.acceptByte('<') {
		token, err := p.consumeUntilByte('>')
		if err != nil {
			return nil, err
		}
		return &Condition{Not: not, Token: token}, nil
	} else if p.acceptByte('[') {
		etag, err := p.consumeUntilByte(']')
		if err != nil {
			return nil, err
		}
		return &Condition{Not: not, ETag: etag}, nil
	} else {
		return nil, fmt.Errorf("webdav: invalid If value: invalid condition")
	}
}

func (p *conditionParser) list() ([]Condition, error) {
	if err := p.expectByte('('); err != nil {
		return nil, err
	}
	p.lws()

	var l []Condition
	for !p.acceptByte(')') {
		cond, err := p.condition()
		if err != nil {
			return nil, err
		}
		l = append(l, *cond)
		p.lws()
	}

	return l, nil
}

func (p *conditionParser) parse() ([][]Condition, error) {
	var conditions [][]Condition
	for {
		p.lws()
		if p.s == "" {
			break
		}

		var resource string
		if p.acceptByte('<') {
			var err error
			resource, err = p.consumeUntilByte('>')
			if err != nil {
				return nil, err
			}
			p.lws()
		}

		l, err := p.list()
		if err != nil {
			return nil, err
		}

		for i := range l {
			l[i].Resource = resource
		}

		conditions = append(conditions, l)
	}

	if len(conditions) == 0 {
		return nil, fmt.Errorf("webdav: invalid If value: empty list")
	}
	return conditions, nil
}

func ParseConditions(s string) ([][]Condition, error) {
	p := conditionParser{s}
	return p.parse()
}

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

func IsNotFound(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Code == http.StatusNotFound
	}
	return false
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

func (err *HTTPError) Unwrap() error {
	return err.Err
}
