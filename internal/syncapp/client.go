package syncapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type LoginResult struct {
	Token string `json:"token"`
}

type ObjectItem struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}

type listObjectsResp struct {
	Objects    []ObjectItem `json:"objects"`
	Truncated  bool         `json:"truncated"`
	NextMarker string       `json:"next_marker"`
}

func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *Client) Login(username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/admin/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Token       string `json:"token"`
		MFARequired bool   `json:"mfa_required"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.MFARequired {
		return "", fmt.Errorf("mfa required — use console login and datasafe-sync token --set")
	}
	if out.Token == "" {
		return "", fmt.Errorf("login failed: %s", out.Error)
	}
	c.Token = out.Token
	return out.Token, nil
}

func (c *Client) authRequest(method, rel string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.BaseURL+rel, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

func (c *Client) ListAllObjects(bucket, prefix string) ([]ObjectItem, error) {
	var all []ObjectItem
	marker := ""
	for {
		q := url.Values{}
		if prefix != "" {
			q.Set("prefix", prefix)
		}
		if marker != "" {
			q.Set("start_after", marker)
		}
		q.Set("max_keys", "1000")
		rel := fmt.Sprintf("/api/v1/buckets/%s/objects?%s", url.PathEscape(bucket), q.Encode())
		req, err := c.authRequest(http.MethodGet, rel, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list objects %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		var page listObjectsResp
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		for _, o := range page.Objects {
			if strings.HasSuffix(o.Key, "/") && o.Size == 0 {
				continue
			}
			all = append(all, o)
		}
		if !page.Truncated || page.NextMarker == "" {
			break
		}
		marker = page.NextMarker
	}
	return all, nil
}

func (c *Client) DownloadObject(bucket, key string) ([]byte, error) {
	rel := fmt.Sprintf("/api/v1/buckets/%s/objects/%s", url.PathEscape(bucket), objectKeyPath(key))
	req, err := c.authRequest(http.MethodGet, rel, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) UploadObject(bucket, key, contentType string, data []byte) error {
	rel := fmt.Sprintf("/api/v1/buckets/%s/objects/%s", url.PathEscape(bucket), objectKeyPath(key))
	req, err := c.authRequest(http.MethodPut, rel, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *Client) DeleteObject(bucket, key string) error {
	rel := fmt.Sprintf("/api/v1/buckets/%s/objects/%s", url.PathEscape(bucket), objectKeyPath(key))
	req, err := c.authRequest(http.MethodDelete, rel, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

type BucketAccess struct {
	Ownership      string   `json:"ownership"`
	CanRead        bool     `json:"can_read"`
	CanWrite       bool     `json:"can_write"`
	SharedBy       string   `json:"shared_by,omitempty"`
	SharedPrefixes []string `json:"shared_prefixes,omitempty"`
}

type BucketInfo struct {
	Name   string        `json:"name"`
	Access *BucketAccess `json:"access,omitempty"`
}

type listBucketsResp struct {
	Buckets []BucketInfo `json:"buckets"`
}

func (c *Client) ListBuckets() ([]BucketInfo, error) {
	req, err := c.authRequest(http.MethodGet, "/api/v1/buckets", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list buckets %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out listBucketsResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Buckets, nil
}

func objectKeyPath(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return path.Join(parts...)
}
