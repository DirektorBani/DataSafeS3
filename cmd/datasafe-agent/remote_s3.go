package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// s3Remote speaks path-style S3 for an access-key profile. Credentials stay
// in memory for the call and are not written to the journal.
type s3Remote struct {
	base      string
	bucket    string
	accessKey string
	secret    string
	region    string
	client    *http.Client
	assemble  *http.Client
	now       func() time.Time
}

func (r *s3Remote) sign(req *http.Request) {
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	s3Sign(req, r.accessKey, r.secret, r.region, now)
}

func (r *s3Remote) do(ctx context.Context, method, path string, query url.Values, body io.Reader, n int64, header map[string]string) (*http.Response, error) {
	return r.doClient(ctx, r.client, method, path, query, body, n, header)
}

func (r *s3Remote) doClient(ctx context.Context, client *http.Client, method, path string, query url.Values, body io.Reader, n int64, header map[string]string) (*http.Response, error) {
	u := strings.TrimRight(r.base, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if body != nil && n >= 0 {
		req.ContentLength = n
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	r.sign(req)
	if client == nil {
		client = r.client
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

func (r *s3Remote) keyPath(key string) string {
	return "/" + url.PathEscape(r.bucket) + "/" + escapeKey(key)
}

func (r *s3Remote) Put(ctx context.Context, key string, body io.Reader, n int64) (string, error) {
	resp, err := r.do(ctx, http.MethodPut, r.keyPath(key), nil, body, n, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", readFault(resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return strings.Trim(resp.Header.Get("ETag"), `"`), nil
}

func (r *s3Remote) Initiate(ctx context.Context, key string) (string, error) {
	resp, err := r.do(ctx, http.MethodPost, r.keyPath(key), url.Values{"uploads": {""}}, nil, -1, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	id := xmlText(raw, "UploadId")
	if id == "" {
		return "", fmt.Errorf("upload id missing")
	}
	return id, nil
}

func (r *s3Remote) Part(ctx context.Context, key, uploadID string, part int, body io.Reader, n int64) (string, error) {
	q := url.Values{}
	q.Set("partNumber", itoa(part))
	q.Set("uploadId", uploadID)
	resp, err := r.do(ctx, http.MethodPut, r.keyPath(key), q, body, n, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", readFault(resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	if etag == "" {
		return "", fmt.Errorf("etag missing")
	}
	return etag, nil
}

func (r *s3Remote) Complete(ctx context.Context, key, uploadID string, parts []partRec) (string, error) {
	var b strings.Builder
	b.WriteString("<CompleteMultipartUpload>")
	for _, part := range sortedParts(parts) {
		etag := strings.Trim(part.ETag, `"`)
		fmt.Fprintf(&b, "<Part><PartNumber>%d</PartNumber><ETag>&quot;%s&quot;</ETag></Part>", part.Number, xmlEscape(etag))
	}
	b.WriteString("</CompleteMultipartUpload>")
	payload := []byte(b.String())
	q := url.Values{}
	q.Set("uploadId", uploadID)
	resp, err := r.doClient(ctx, r.assemble, http.MethodPost, r.keyPath(key), q, bytes.NewReader(payload), int64(len(payload)), map[string]string{"Content-Type": "application/xml"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	return strings.Trim(xmlText(raw, "ETag"), `"`), nil
}

func (r *s3Remote) Stat(ctx context.Context, key string) (string, int64, error) {
	resp, err := r.do(ctx, http.MethodHead, r.keyPath(key), nil, nil, -1, nil)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", 0, readFault(resp)
	}
	return strings.Trim(resp.Header.Get("ETag"), `"`), resp.ContentLength, nil
}

func (r *s3Remote) Abort(ctx context.Context, key, uploadID string) error {
	q := url.Values{}
	q.Set("uploadId", uploadID)
	resp, err := r.do(ctx, http.MethodDelete, r.keyPath(key), q, nil, -1, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 400 {
		return nil
	}
	return readFault(resp)
}

func (r *s3Remote) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error) {
	header := map[string]string{}
	if offset > 0 {
		header["Range"] = fmt.Sprintf("bytes=%d-", offset)
	}
	resp, err := r.do(ctx, http.MethodGet, r.keyPath(key), nil, nil, -1, header)
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

func (r *s3Remote) List(ctx context.Context, prefix, startAfter string) ([]listedObject, bool, string, error) {
	q := url.Values{}
	q.Set("list-type", "2")
	q.Set("prefix", prefix)
	q.Set("max-keys", "1000")
	if startAfter != "" {
		q.Set("start-after", startAfter)
	}
	resp, err := r.do(ctx, http.MethodGet, "/"+url.PathEscape(r.bucket), q, nil, -1, nil)
	if err != nil {
		return nil, false, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, false, "", faultFrom(resp.StatusCode, raw, resp.Header)
	}
	var doc struct {
		Truncated bool `xml:"IsTruncated"`
		Contents  []struct {
			Key  string `xml:"Key"`
			Size int64  `xml:"Size"`
			ETag string `xml:"ETag"`
		} `xml:"Contents"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, false, "", err
	}
	items := make([]listedObject, 0, len(doc.Contents))
	var last string
	for _, obj := range doc.Contents {
		if obj.Key == "" || strings.HasSuffix(obj.Key, "/") {
			continue
		}
		items = append(items, listedObject{Key: obj.Key, Size: obj.Size, ETag: strings.Trim(obj.ETag, `"`)})
		last = obj.Key
	}
	return items, doc.Truncated, last, nil
}

func xmlText(raw []byte, tag string) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != tag {
			continue
		}
		var text string
		if err := dec.DecodeElement(&text, &start); err != nil {
			return ""
		}
		return text
	}
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func sortedParts(parts []partRec) []partRec {
	sorted := append([]partRec(nil), parts...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Number < sorted[i].Number {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}
