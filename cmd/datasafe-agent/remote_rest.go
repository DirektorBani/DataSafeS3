package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type listedObject struct {
	Key  string
	Size int64
	ETag string
}

type apiFault struct {
	Status int
	Code   string
	Body   string
}

func (e *apiFault) Error() string {
	if e.Code != "" {
		return e.Code
	}
	if e.Status != 0 {
		return fmt.Sprintf("status %d", e.Status)
	}
	return "request failed"
}

func faultNotFound(err error) bool {
	f, ok := err.(*apiFault)
	if !ok {
		return false
	}
	body := strings.ToLower(f.Body)
	return f.Status == http.StatusNotFound || f.Code == "multipart.upload_not_found" || strings.Contains(body, "upload not found")
}

func faultETag(err error) bool {
	f, ok := err.(*apiFault)
	if !ok || f.Status < 400 {
		return false
	}
	blob := strings.ToLower(f.Code + " " + f.Body)
	return strings.Contains(blob, "etag")
}

type restRemote struct {
	client    *Client
	bucket    string
	rangeREST bool
}

func (r *restRemote) objectPath(key string) string {
	return "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/objects/" + escapeKey(key)
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (r *restRemote) Put(ctx context.Context, key string, body io.Reader, n int64) (string, error) {
	resp, err := r.client.stream(ctx, http.MethodPut, r.objectPath(key), body, n, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", readFault(resp)
	}
	return etagFrom(resp)
}

func (r *restRemote) Initiate(ctx context.Context, key string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"key": key, "content_type": "application/octet-stream"})
	resp, err := r.client.stream(ctx, http.MethodPost, "/api/v1/buckets/"+url.PathEscape(r.bucket)+"/multipart", bytes.NewReader(payload), int64(len(payload)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.UploadID == "" {
		return "", fmt.Errorf("upload id missing")
	}
	return out.UploadID, nil
}

func (r *restRemote) Part(ctx context.Context, key, uploadID string, part int, body io.Reader, n int64) (string, error) {
	path := "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/multipart/" + url.PathEscape(uploadID) + "/parts/" + itoa(part)
	resp, err := r.client.stream(ctx, http.MethodPut, path, body, n, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		ETag string `json:"etag"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.ETag == "" {
		out.ETag = strings.Trim(resp.Header.Get("ETag"), `"`)
	}
	if out.ETag == "" {
		return "", fmt.Errorf("etag missing")
	}
	return strings.Trim(out.ETag, `"`), nil
}

func (r *restRemote) Complete(ctx context.Context, key, uploadID string, parts []partRec) (string, error) {
	body := map[string]any{"parts": partsJSON(parts)}
	payload, _ := json.Marshal(body)
	path := "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/multipart/" + url.PathEscape(uploadID) + "/complete"
	resp, err := r.client.streamAssemble(ctx, http.MethodPost, path, bytes.NewReader(payload), int64(len(payload)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		Object struct {
			ETag string `json:"etag"`
		} `json:"object"`
	}
	_ = json.Unmarshal(raw, &out)
	return strings.Trim(out.Object.ETag, `"`), nil
}

func (r *restRemote) Stat(ctx context.Context, key string) (string, int64, error) {
	path := "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/object-meta?key=" + url.QueryEscape(key)
	resp, err := r.client.stream(ctx, http.MethodGet, path, nil, -1, nil)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", 0, faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		Object struct {
			ETag string `json:"etag"`
			Size int64  `json:"size"`
		} `json:"object"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", 0, err
	}
	return strings.Trim(out.Object.ETag, `"`), out.Object.Size, nil
}

func (r *restRemote) Abort(ctx context.Context, key, uploadID string) error {
	path := "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/multipart/" + url.PathEscape(uploadID)
	resp, err := r.client.stream(ctx, http.MethodDelete, path, nil, -1, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return readFault(resp)
}

func (r *restRemote) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error) {
	if offset > 0 {
		if r.rangeREST {
			return r.openHeaders(ctx, r.objectPath(key), offset, true)
		}
		return r.openPresigned(ctx, key, offset)
	}
	return r.openHeaders(ctx, r.objectPath(key), 0, true)
}

func (r *restRemote) openPresigned(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error) {
	payload, _ := json.Marshal(map[string]any{
		"method":          http.MethodGet,
		"bucket":          r.bucket,
		"key":             key,
		"expires_seconds": 3600,
	})
	resp, err := r.client.stream(ctx, http.MethodPost, "/api/v1/presign", bytes.NewReader(payload), int64(len(payload)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return nil, "", 0, 0, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, "", 0, resp.StatusCode, errNoRange
	}
	if resp.StatusCode >= 400 {
		return nil, "", 0, resp.StatusCode, faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.URL == "" {
		return nil, "", 0, resp.StatusCode, errNoRange
	}
	return r.client.openURL(ctx, out.URL, offset)
}

func (r *restRemote) openHeaders(ctx context.Context, path string, offset int64, bearer bool) (io.ReadCloser, string, int64, int, error) {
	header := map[string]string{}
	if offset > 0 {
		header["Range"] = fmt.Sprintf("bytes=%d-", offset)
	}
	resp, err := r.client.stream(ctx, http.MethodGet, path, nil, -1, header)
	if err != nil {
		return nil, "", 0, 0, err
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

func (r *restRemote) List(ctx context.Context, prefix, startAfter string) ([]listedObject, bool, string, error) {
	query := url.Values{}
	query.Set("prefix", prefix)
	query.Set("max_keys", "1000")
	if startAfter != "" {
		query.Set("start_after", startAfter)
	}
	path := "/api/v1/buckets/" + url.PathEscape(r.bucket) + "/objects?" + query.Encode()
	resp, err := r.client.stream(ctx, http.MethodGet, path, nil, -1, nil)
	if err != nil {
		return nil, false, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, false, "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var out struct {
		Objects []struct {
			Key  string `json:"key"`
			Size int64  `json:"size"`
			ETag string `json:"etag"`
		} `json:"objects"`
		Truncated bool   `json:"truncated"`
		Next      string `json:"next_marker"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false, "", err
	}
	items := make([]listedObject, 0, len(out.Objects))
	for _, obj := range out.Objects {
		if obj.Key == "" || strings.HasSuffix(obj.Key, "/") {
			continue
		}
		items = append(items, listedObject{Key: obj.Key, Size: obj.Size, ETag: strings.Trim(obj.ETag, `"`)})
	}
	return items, out.Truncated, out.Next, nil
}

func partsJSON(parts []partRec) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	sorted := append([]partRec(nil), parts...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Number < sorted[i].Number {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for _, part := range sorted {
		out = append(out, map[string]any{"part_number": part.Number, "etag": strings.Trim(part.ETag, `"`)})
	}
	return out
}

func etagFrom(resp *http.Response) (string, error) {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Object struct {
			ETag string `json:"etag"`
		} `json:"object"`
		ETag string `json:"etag"`
	}
	_ = json.Unmarshal(raw, &out)
	etag := out.Object.ETag
	if etag == "" {
		etag = out.ETag
	}
	if etag == "" {
		etag = resp.Header.Get("ETag")
	}
	return strings.Trim(etag, `"`), nil
}

func readFault(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return faultFrom(resp.StatusCode, raw, resp.Header)
}

func faultFrom(status int, raw []byte, header http.Header) error {
	apiErr := classifyError(status, raw, header)
	return &apiFault{Status: status, Code: apiErr.Code, Body: string(raw)}
}
