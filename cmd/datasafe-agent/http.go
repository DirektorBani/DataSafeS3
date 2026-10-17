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
	return c.doHeaders(method, path, payload, nil)
}

// doHeaders is do with extra headers. The step-up retry sets
// X-DataSafe-MFA-Token here and nowhere else. The token is not logged.
func (c *Client) doHeaders(method, path string, payload any, headers map[string]string) (rawResponse, error) {
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
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
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
	if err := classifyStatus(offset, resp); err != nil {
		resp.Body.Close()
		return nil, "", 0, resp.StatusCode, err
	}
	return resp.Body, strings.Trim(resp.Header.Get("ETag"), `"`), resp.ContentLength, resp.StatusCode, nil
}

// classifyStatus separates a ranged GET that the server answered in full
// from a failure that should be retried or reported. A 5xx or 429 keeps the
// download offset. A 200 where a range was requested is the only "no range"
// signal. A 4xx is the fault itself, not a reason to start the file over.
func classifyStatus(offset int64, resp *http.Response) error {
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		fault := readFault(resp)
		return fmt.Errorf("%w: %s", errNetwork, publicError(fault.Error()))
	}
	if resp.StatusCode >= 400 {
		return readFault(resp)
	}
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		return errNoRange
	}
	return nil
}

// netReason classifies a dial failure. A bad certificate is not the same
// as a host that never answered.
func netReason(err error) string {
	if err == nil {
		return ""
	}
	var tlsFail *tlsError
	if errors.As(err, &tlsFail) {
		return "certificate"
	}
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var roots x509.SystemRootsError
	if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &roots) {
		return "certificate"
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "certificate") || strings.Contains(text, "x509:") {
		return "certificate"
	}
	return "unreachable"
}

func preferNetReason(reasons ...string) string {
	for _, reason := range reasons {
		if reason == "certificate" {
			return reason
		}
	}
	for _, reason := range reasons {
		if reason != "" {
			return reason
		}
	}
	return ""
}

// wrapNet turns a transport failure into errNetwork. Go's
// ResponseHeaderTimeout error implements Is(context.DeadlineExceeded) and is
// wrapped in url.Error, whose own Error() includes the full request URL.
// Returning that value made a header timeout fatal and stored the presigned
// query string. A real cancel stays context.Canceled. A header timeout, an
// i/o timeout, and a connection reset are network waits: the task retries
// from its offset or confirmed parts.
func wrapNet(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	text := publicError(err.Error())
	// A certificate failure does not clear itself. Retrying it forever
	// leaves the task in waiting_network. Timeouts, resets, and a host
	// that does not answer are the network wait. The task stores a short
	// code. The x509 text, without the request URL, is only for the log.
	if netReason(err) == "certificate" {
		return &tlsError{code: tlsCode(err), detail: tlsDetail(err)}
	}
	return fmt.Errorf("%w: %s", errNetwork, text)
}

// tlsError is the task-facing certificate failure. Error is the short code.
// detail is the x509 reason with no request URL.
type tlsError struct {
	code   string
	detail string
}

func (e *tlsError) Error() string {
	if e == nil || e.code == "" {
		return "tls.failed"
	}
	return e.code
}

func tlsCode(err error) string {
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return "tls.untrusted"
	}
	if strings.Contains(strings.ToLower(err.Error()), "unknown authority") {
		return "tls.untrusted"
	}
	return "tls.failed"
}

func tlsDetail(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		inner := uerr.Err.Error()
		if !strings.Contains(inner, "://") {
			return inner
		}
	}
	return strings.TrimSpace(logURL.ReplaceAllString(err.Error(), ""))
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
