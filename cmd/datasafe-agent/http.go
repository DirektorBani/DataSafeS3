package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client talks to one DataSafeS3 origin. Secrets stay in the request body
// and the Authorization header; they are not written to the log.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// Stream has no overall timeout. Transfer bodies are larger than the
	// JSON client allows, and a 10 GiB copy must not be cut off at 20s.
	// ResponseHeaderTimeout still bounds how long a request waits for
	// headers after its body has been written. That is fine for a part
	// upload. It is not fine for CompleteMultipartUpload: server 1.6.1
	// assembles at roughly 16–22s per GiB, so a multi-gigabyte complete
	// never returns headers inside two minutes.
	Stream *http.Client
	// Assemble is the client for CompleteMultipartUpload. It has no
	// response-header timeout. A dropped connection is retried; the
	// upload id is kept.
	Assemble *http.Client
}

func newClient(baseURL string, insecure bool) (*Client, error) {
	return newClientConfig(baseURL, insecure, "")
}

func clientFor(profile Profile) (*Client, error) {
	return newClientConfig(profile.ServerURL, profile.TLS.InsecureDev, profile.TLS.CAFile)
}

func newClientConfig(baseURL string, insecure bool, caFile string) (*Client, error) {
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
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(caFile) != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("ca file is invalid")
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca file is invalid")
		}
		tlsCfg.RootCAs = pool
	}
	if insecure {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // profile flag insecure_dev
	}
	transport.TLSClientConfig = tlsCfg
	streamTransport := transport.Clone()
	streamTransport.ResponseHeaderTimeout = 2 * time.Minute
	assembleTransport := transport.Clone()
	noRedirect := func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{
		BaseURL: strings.TrimRight(u.String(), "/"),
		HTTP: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
		Stream: &http.Client{
			Timeout:       0,
			Transport:     streamTransport,
			CheckRedirect: noRedirect,
		},
		Assemble: &http.Client{
			Timeout:       0,
			Transport:     assembleTransport,
			CheckRedirect: noRedirect,
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

// stream sends a body without the 1 MiB JSON cap. The caller closes the
// response body. A dropped connection is errNetwork.
func (c *Client) stream(ctx context.Context, method, path string, body io.Reader, n int64, header map[string]string) (*http.Response, error) {
	return c.streamOn(ctx, c.Stream, method, path, body, n, header)
}

// streamAssemble is stream for CompleteMultipartUpload. The assemble client
// waits for the server to finish joining parts.
func (c *Client) streamAssemble(ctx context.Context, method, path string, body io.Reader, n int64, header map[string]string) (*http.Response, error) {
	client := c.Assemble
	if client == nil {
		client = c.Stream
	}
	return c.streamOn(ctx, client, method, path, body, n, header)
}

func (c *Client) streamOn(ctx context.Context, client *http.Client, method, path string, body io.Reader, n int64, header map[string]string) (*http.Response, error) {
	if client == nil {
		client = c.Stream
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil && n >= 0 {
		req.ContentLength = n
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, wrapNet(err)
	}
	return resp, nil
}

// openURL fetches a presigned URL. The bearer token is not sent: the
// signature is in the query string.
func (c *Client) openURL(ctx context.Context, rawURL string, offset int64) (io.ReadCloser, string, int64, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.Stream.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", 0, 0, ctx.Err()
		}
		return nil, "", 0, 0, wrapNet(err)
	}
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, "", 0, resp.StatusCode, errNoRange
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, "", 0, resp.StatusCode, readFault(resp)
	}
	return resp.Body, strings.Trim(resp.Header.Get("ETag"), `"`), resp.ContentLength, resp.StatusCode, nil
}

func wrapNet(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil && (errors.Is(ue.Err, context.Canceled) || errors.Is(ue.Err, context.DeadlineExceeded)) {
		return ue.Err
	}
	return fmt.Errorf("%w: %s", errNetwork, err.Error())
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
