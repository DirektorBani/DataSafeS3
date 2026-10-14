package migrate

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
)

// ErrNotLeader is a write that landed on a follower.
var ErrNotLeader = errors.New("not cluster leader")

func (e Endpoint) httpClient() *http.Client {
	if e.Region == "" {
		e.Region = "us-east-1"
	}
	return &http.Client{Timeout: 0}
}

func (e Endpoint) do(ctx context.Context, method, path string, q url.Values, body io.Reader, hdr http.Header) (*http.Response, error) {
	u, err := url.Parse(strings.TrimRight(e.URL, "/") + path)
	if err != nil {
		return nil, err
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	region := e.Region
	if region == "" {
		region = "us-east-1"
	}
	if err := auth.SignRequest(req, auth.Credentials{AccessKey: e.AccessKey, SecretKey: e.SecretKey}, region, "s3", "UNSIGNED-PAYLOAD"); err != nil {
		return nil, err
	}
	return e.httpClient().Do(req)
}

func objectPath(bucket, key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/" + url.PathEscape(bucket) + "/" + strings.Join(parts, "/")
}

type listedVersion struct {
	XMLName      xml.Name
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
}

type listVersionsResult struct {
	XMLName             xml.Name        `xml:"ListVersionsResult"`
	IsTruncated         bool            `xml:"IsTruncated"`
	NextKeyMarker       string          `xml:"NextKeyMarker"`
	NextVersionIDMarker string          `xml:"NextVersionIdMarker"`
	Versions            []listedVersion `xml:"Version"`
	DeleteMarkers       []listedVersion `xml:"DeleteMarker"`
}

type listBucketsResult struct {
	Buckets []struct {
		Name string `xml:"Name"`
	} `xml:"Buckets>Bucket"`
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.RFC1123} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func trimETag(s string) string {
	return strings.Trim(s, `"`)
}

func readErr(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return strings.TrimSpace(string(b))
}

func isNotLeader(status int, body string) bool {
	return status == http.StatusServiceUnavailable && strings.Contains(strings.ToLower(body), "not cluster leader")
}
