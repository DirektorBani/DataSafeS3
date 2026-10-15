package vaulttransit

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
)

const connectionTestKey = "datasafe-connection-test"

// Typed failures for the settings test button. Data-path callers still fail closed.
var (
	ErrBadSecret    = errors.New("bad secret id")
	ErrNoPermission = errors.New("no transit permission")
	ErrTimeout      = errors.New("vault timeout")
)

// StatusError is a Vault HTTP error with the token and secret already removed.
type StatusError struct {
	Op     string
	Status int
	Detail string
}

func (e *StatusError) Error() string {
	if e == nil {
		return ""
	}
	if e.Detail == "" {
		return e.Op
	}
	return e.Op + ": " + e.Detail
}

// Client is a Transit REST client. It does not use the Vault SDK.
type Client struct {
	cfg   Config
	http  *http.Client
	mu    sync.Mutex
	token string
	exp   time.Time
	// seen holds credential bytes so a Vault error body cannot echo them.
	seen []string
	wait chan struct{}
}

func newClient(cfg Config) (*Client, error) {
	cfg.applyDefaults()
	if !cfg.Enabled() {
		return nil, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(cfg.CACert) != "" {
		pool, err := caPool(cfg.CACert)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func caPool(ca string) (*x509.CertPool, error) {
	pemBytes := []byte(ca)
	if !strings.Contains(ca, "BEGIN CERTIFICATE") {
		b, err := os.ReadFile(strings.TrimSpace(ca))
		if err != nil {
			return nil, errors.New("vault CA certificate could not be read")
		}
		pemBytes = b
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("vault CA certificate is not valid PEM")
	}
	return pool, nil
}

func (c *Client) remember(secrets ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range secrets {
		if s != "" {
			c.seen = append(c.seen, s)
		}
	}
}

func (c *Client) scrub(msg string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.seen {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "redacted")
		}
	}
	return msg
}

func (c *Client) fail(op string, status int, detail string) error {
	return &StatusError{Op: op, Status: status, Detail: c.scrub(detail)}
}

// Classify maps a client error to a settings-test category.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrTimeout) {
		return ErrTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ErrTimeout
	}
	var se *StatusError
	if errors.As(err, &se) {
		low := strings.ToLower(se.Detail)
		if se.Status == 0 || strings.Contains(low, "timeout") || strings.Contains(low, "connection refused") || strings.Contains(low, "no such host") {
			return ErrTimeout
		}
		if se.Op == "login" && (se.Status == 400 || se.Status == 403 || strings.Contains(low, "secret") || strings.Contains(low, "invalid role")) {
			return ErrBadSecret
		}
		if se.Status == 403 || strings.Contains(low, "permission denied") {
			return ErrNoPermission
		}
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "timeout") || strings.Contains(low, "connection refused") || strings.Contains(low, "no such host") {
		return ErrTimeout
	}
	return kms.ErrUnavailable
}

func (c *Client) endpoint(p string) string {
	return c.cfg.Addr + p
}

func (c *Client) mountPath(suffix string) string {
	mount := strings.Trim(c.cfg.Mount, "/")
	return "/v1/" + mount + "/" + suffix
}

func (c *Client) do(ctx context.Context, method, path, op string, in, out any, token string) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return c.fail(op, 0, err.Error())
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if c.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.cfg.Namespace)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return c.fail(op, 0, err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return c.fail(op, resp.StatusCode, vaultErrText(raw))
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return c.fail(op, resp.StatusCode, "invalid vault response")
		}
	}
	return nil
}

func vaultErrText(raw []byte) string {
	var body struct {
		Errors []string `json:"errors"`
	}
	if json.Unmarshal(raw, &body) == nil && len(body.Errors) > 0 {
		s := strings.Join(body.Errors, "; ")
		if len(s) > 180 {
			return s[:180]
		}
		return s
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 180 {
		return s[:180]
	}
	return s
}

func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Until(c.exp) > 30*time.Second {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	if c.wait != nil {
		ch := c.wait
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", c.fail("login", 0, ctx.Err().Error())
		}
		return c.ensureToken(ctx)
	}
	ch := make(chan struct{})
	c.wait = ch
	c.mu.Unlock()

	err := c.login(ctx)
	c.mu.Lock()
	c.wait = nil
	tok := c.token
	close(ch)
	c.mu.Unlock()
	if err != nil {
		return "", err
	}
	return tok, nil
}

func (c *Client) login(ctx context.Context) error {
	if c.cfg.Token != "" || c.cfg.TokenFile != "" {
		tok, err := readSecret(c.cfg.Token, c.cfg.TokenFile)
		if err != nil || tok == "" {
			return c.fail("login", 0, "token is empty")
		}
		c.remember(tok)
		if err := c.renew(ctx, tok); err != nil {
			if !keepStaticToken(err) {
				c.mu.Lock()
				c.token = ""
				c.exp = time.Time{}
				c.mu.Unlock()
				return err
			}
			c.mu.Lock()
			c.token = tok
			c.exp = time.Now().Add(30 * time.Minute)
			c.mu.Unlock()
		}
		return nil
	}
	secret, err := readSecret(c.cfg.SecretID, c.cfg.SecretIDFile)
	if err != nil {
		return c.fail("login", 0, "secret id file could not be read")
	}
	if strings.TrimSpace(c.cfg.RoleID) == "" || secret == "" {
		return c.fail("login", 400, "role id or secret id is empty")
	}
	c.remember(secret, c.cfg.RoleID)
	c.mu.Lock()
	old := c.token
	c.mu.Unlock()
	if old != "" {
		if err := c.renew(ctx, old); err == nil {
			return nil
		}
	}
	var resp struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	err = c.do(ctx, http.MethodPost, "/v1/auth/approle/login", "login", map[string]string{
		"role_id":   c.cfg.RoleID,
		"secret_id": secret,
	}, &resp, "")
	if err != nil {
		return err
	}
	if resp.Auth.ClientToken == "" {
		return c.fail("login", 400, "vault returned no token")
	}
	c.remember(resp.Auth.ClientToken)
	lease := resp.Auth.LeaseDuration
	if lease < 60 {
		lease = 60
	}
	c.mu.Lock()
	c.token = resp.Auth.ClientToken
	c.exp = time.Now().Add(time.Duration(lease) * time.Second)
	c.mu.Unlock()
	return nil
}

// keepStaticToken reports whether a renew failure still leaves a configured token usable.
// Dev-mode root tokens, and other tokens with no TTL, answer 400 "token not renewable".
// HTTP 403 and any other 400 (expired, unknown) reject the token.
func keepStaticToken(err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return true
	}
	if se.Status == http.StatusForbidden {
		return false
	}
	low := strings.ToLower(se.Detail)
	if strings.Contains(low, "not renewable") {
		return true
	}
	if se.Status == http.StatusBadRequest {
		return false
	}
	return true
}

func (c *Client) renew(ctx context.Context, tok string) error {
	var resp struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/auth/token/renew-self", "renew", map[string]string{}, &resp, tok)
	if err != nil {
		return err
	}
	next := resp.Auth.ClientToken
	if next == "" {
		next = tok
	}
	lease := resp.Auth.LeaseDuration
	if lease < 60 {
		lease = 60
	}
	c.remember(next)
	c.mu.Lock()
	c.token = next
	c.exp = time.Now().Add(time.Duration(lease) * time.Second)
	c.mu.Unlock()
	return nil
}

func readSecret(inline, path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(inline), nil
}

// Health checks that Vault answers and this node's credentials still log in.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/sys/health"), nil)
	if err != nil {
		return c.fail("health", 0, err.Error())
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return c.fail("health", 0, err.Error())
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// 200 active, 429 standby, 472 DR secondary, 473 performance standby.
	// A sealed or uninitialized server is not available.
	switch resp.StatusCode {
	case http.StatusOK, http.StatusTooManyRequests, 472, 473:
	default:
		return c.fail("health", resp.StatusCode, "vault unavailable")
	}
	_, err = c.ensureToken(ctx)
	return err
}

// EnsureKey creates an aes256-gcm96 Transit key when it is missing.
func (c *Client) EnsureKey(ctx context.Context, name string) error {
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}
	path := c.mountPath("keys/" + url.PathEscape(name))
	err = c.do(ctx, http.MethodPost, path, "create", map[string]string{"type": "aes256-gcm96"}, nil, tok)
	if err != nil {
		var se *StatusError
		if !errors.As(err, &se) || (se.Status != http.StatusNoContent && se.Status != http.StatusBadRequest) {
			return err
		}
		if gerr := c.do(ctx, http.MethodGet, path, "read", nil, &struct{}{}, tok); gerr != nil {
			return err
		}
	}
	// Transit refuses DELETE until deletion_allowed is set. Set it when the key
	// is created so crypto-shred can remove the key with the admin role.
	return c.do(ctx, http.MethodPost, path+"/config", "config", map[string]bool{"deletion_allowed": true}, nil, tok)
}

// Encrypt seals plaintext and returns the Transit ciphertext.
func (c *Client) Encrypt(ctx context.Context, name string, plain []byte) (string, int, error) {
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return "", 0, err
	}
	var resp struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
			KeyVersion int    `json:"key_version"`
		} `json:"data"`
	}
	err = c.do(ctx, http.MethodPost, c.mountPath("encrypt/"+url.PathEscape(name)), "encrypt", map[string]string{
		"plaintext": base64.StdEncoding.EncodeToString(plain),
	}, &resp, tok)
	if err != nil {
		return "", 0, err
	}
	ver := resp.Data.KeyVersion
	if ver < 1 {
		ver = cipherVersion(resp.Data.Ciphertext)
	}
	return resp.Data.Ciphertext, ver, nil
}

// Decrypt opens a Transit ciphertext.
func (c *Client) Decrypt(ctx context.Context, name, ciphertext string) ([]byte, error) {
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	err = c.do(ctx, http.MethodPost, c.mountPath("decrypt/"+url.PathEscape(name)), "decrypt", map[string]string{
		"ciphertext": ciphertext,
	}, &resp, tok)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Data.Plaintext)
}

// Rewrap asks Transit for a ciphertext under the latest key version.
func (c *Client) Rewrap(ctx context.Context, name, ciphertext string) (string, int, error) {
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return "", 0, err
	}
	var resp struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
			KeyVersion int    `json:"key_version"`
		} `json:"data"`
	}
	err = c.do(ctx, http.MethodPost, c.mountPath("rewrap/"+url.PathEscape(name)), "rewrap", map[string]string{
		"ciphertext": ciphertext,
	}, &resp, tok)
	if err != nil {
		return "", 0, err
	}
	ver := resp.Data.KeyVersion
	if ver < 1 {
		ver = cipherVersion(resp.Data.Ciphertext)
	}
	return resp.Data.Ciphertext, ver, nil
}

// Rotate bumps the Transit key version. It uses the admin AppRole when one is set.
func (c *Client) Rotate(ctx context.Context, name string) (int, error) {
	tok, err := c.privilegedToken(ctx)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Data struct {
			LatestVersion int `json:"latest_version"`
		} `json:"data"`
	}
	err = c.do(ctx, http.MethodPost, c.mountPath("keys/"+url.PathEscape(name)+"/rotate"), "rotate", map[string]string{}, &resp, tok)
	if err != nil {
		return 0, err
	}
	if resp.Data.LatestVersion > 0 {
		return resp.Data.LatestVersion, nil
	}
	return 0, nil
}

// DeleteKey removes a Transit key. Tests and crypto-shred use it.
func (c *Client) DeleteKey(ctx context.Context, name string) error {
	tok, err := c.privilegedToken(ctx)
	if err != nil {
		return err
	}
	path := c.mountPath("keys/" + url.PathEscape(name))
	cfgErr := c.do(ctx, http.MethodPost, path+"/config", "config", map[string]bool{"deletion_allowed": true}, nil, tok)
	delErr := c.do(ctx, http.MethodDelete, path, "delete", nil, nil, tok)
	if delErr != nil {
		if cfgErr != nil {
			return c.fail("delete", 0, delErr.Error()+"; deletion_allowed: "+cfgErr.Error())
		}
		return delErr
	}
	return nil
}

func (c *Client) privilegedToken(ctx context.Context) (string, error) {
	if c.cfg.AdminRoleID == "" && c.cfg.AdminSecretID == "" && c.cfg.AdminSecretIDFile == "" {
		return c.ensureToken(ctx)
	}
	secret, err := readSecret(c.cfg.AdminSecretID, c.cfg.AdminSecretIDFile)
	if err != nil || secret == "" || c.cfg.AdminRoleID == "" {
		return "", c.fail("login", 400, "admin role is incomplete")
	}
	c.remember(secret, c.cfg.AdminRoleID)
	var resp struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	err = c.do(ctx, http.MethodPost, "/v1/auth/approle/login", "login", map[string]string{
		"role_id":   c.cfg.AdminRoleID,
		"secret_id": secret,
	}, &resp, "")
	if err != nil {
		return "", err
	}
	c.remember(resp.Auth.ClientToken)
	return resp.Auth.ClientToken, nil
}

// TestLoginEncrypt checks login plus encrypt and decrypt, then deletes the
// temporary Transit key so a connection test does not leave datasafe-connection-test.
func (c *Client) TestLoginEncrypt(ctx context.Context) (err error) {
	if err = c.EnsureKey(ctx, connectionTestKey); err != nil {
		return err
	}
	defer func() {
		if delErr := c.DeleteKey(ctx, connectionTestKey); delErr != nil && err == nil {
			err = delErr
		}
	}()
	ct, _, encErr := c.Encrypt(ctx, connectionTestKey, []byte("datasafe"))
	if encErr != nil {
		return encErr
	}
	plain, decErr := c.Decrypt(ctx, connectionTestKey, ct)
	if decErr != nil {
		return decErr
	}
	if string(plain) != "datasafe" {
		return c.fail("decrypt", 200, "transit decrypt mismatch")
	}
	return nil
}

func cipherVersion(ct string) int {
	const prefix = "vault:v"
	if !strings.HasPrefix(ct, prefix) {
		return 1
	}
	rest := ct[len(prefix):]
	n, _, _ := strings.Cut(rest, ":")
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 {
		return 1
	}
	return v
}
