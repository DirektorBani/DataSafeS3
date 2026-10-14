package migrate

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// decodeVersionPage reads Version and DeleteMarker elements in document order.
// seq is the last sequence number already assigned; the next page continues it.
func decodeVersionPage(r io.Reader, bucket string, seq int) (truncated bool, nextKey, nextVid string, items []ObjectVersion, next int, err error) {
	dec := xml.NewDecoder(r)
	var cur *ObjectVersion
	var stack []string
	next = seq
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return truncated, nextKey, nextVid, items, next, nil
		}
		if err != nil {
			return false, "", "", nil, seq, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if t.Name.Local == "Version" || t.Name.Local == "DeleteMarker" {
				next++
				cur = &ObjectVersion{Bucket: bucket, DeleteMarker: t.Name.Local == "DeleteMarker", Seq: next}
			}
		case xml.EndElement:
			if (t.Name.Local == "Version" || t.Name.Local == "DeleteMarker") && cur != nil {
				items = append(items, *cur)
				cur = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || len(stack) == 0 {
				continue
			}
			field := stack[len(stack)-1]
			if cur != nil {
				switch field {
				case "Key":
					cur.Key = text
				case "VersionId":
					cur.VersionID = text
				case "ETag":
					cur.ETag = trimETag(text)
				case "Size":
					cur.Size, _ = strconv.ParseInt(text, 10, 64)
				case "LastModified":
					cur.LastModified = parseTime(text)
				case "IsLatest":
					cur.IsLatest = strings.EqualFold(text, "true")
				}
				continue
			}
			switch field {
			case "IsTruncated":
				truncated = strings.EqualFold(text, "true")
			case "NextKeyMarker":
				nextKey = text
			case "NextVersionIdMarker":
				nextVid = text
			}
		}
	}
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
