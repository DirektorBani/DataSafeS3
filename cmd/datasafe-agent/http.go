package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one DataSafeS3 origin. Secrets stay in the request body
// and the Authorization header; they are not written to the log.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func newClient(baseURL string, insecure bool) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("server url is invalid")
	}
	if u.Scheme != "https" && !loopback(u) {
		return nil, fmt.Errorf("server url must be https, or http on loopback")
	}
	if u.User != nil {
		return nil, fmt.Errorf("server url must not carry credentials")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // profile flag insecure_dev
	}
	return &Client{
		BaseURL: strings.TrimRight(u.String(), "/"),
		HTTP: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
	}, nil
}

func loopback(u *url.URL) bool {
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return u.Scheme == "http"
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() && u.Scheme == "http"
}

type rawResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

func (c *Client) do(method, path string, payload any) (rawResponse, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return rawResponse{}, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return rawResponse{}, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return rawResponse{}, err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return rawResponse{}, err
	}
	return rawResponse{Status: resp.StatusCode, Header: resp.Header, Body: buf}, nil
}

func decodeObject(raw []byte) (map[string]any, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}
