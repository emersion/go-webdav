package webdav

import (
	"bytes"
	"io"
	"net/http"

	"filippo.io/age"
)

type encryptedHTTPClient struct {
	c   HTTPClient
	id  *age.X25519Identity
	rec *age.X25519Recipient
}

func (c *encryptedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	encBody := &bytes.Buffer{}
	w, err := age.Encrypt(encBody, c.rec)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(w, req.Body); err != nil {
		return nil, err
	}

	encReq := *req
	encReq.Body = io.NopCloser(encBody)

	encResp, err := c.c.Do(&encReq)
	if err != nil {
		return nil, err
	}

	resp := *encResp
	decBody, err := age.Decrypt(resp.Body, c.id)
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(decBody)

	return &resp, nil
}
