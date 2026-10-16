package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// s3Sign adds a SigV4 header. Payload hash UNSIGNED-PAYLOAD matches the
// server's accepted streaming uploads. The clock is injectable for tests.
func s3Sign(r *http.Request, accessKey, secret, region string, now time.Time) {
	if region == "" {
		region = "us-east-1"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	if r.Host == "" {
		r.Host = r.URL.Host
	}
	signed := s3SignedHeaders(r)
	canonical := s3Canonical(r, signed)
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		s3Hash(canonical),
	}, "\n")
	sig := s3Signature(secret, dateStamp, region, stringToSign)
	r.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, strings.Join(signed, ";"), sig,
	))
}

func s3SignedHeaders(r *http.Request) []string {
	set := map[string]struct{}{"host": {}}
	for name := range r.Header {
		ln := strings.ToLower(name)
		if strings.HasPrefix(ln, "x-amz-") || ln == "content-type" || ln == "content-md5" {
			set[ln] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func s3Canonical(r *http.Request, signed []string) string {
	var b strings.Builder
	b.WriteString(r.Method)
	b.WriteByte('\n')
	b.WriteString(s3EncodePath(r.URL.EscapedPath()))
	b.WriteByte('\n')
	b.WriteString(s3CanonicalQuery(r.URL.Query()))
	b.WriteByte('\n')
	b.WriteString(s3CanonicalHeaders(r, signed))
	b.WriteByte('\n')
	b.WriteString(strings.Join(signed, ";"))
	b.WriteByte('\n')
	b.WriteString(r.Header.Get("X-Amz-Content-Sha256"))
	return b.String()
}

func s3CanonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, s3EncodeQuery(k)+"="+s3EncodeQuery(v))
		}
	}
	return strings.Join(parts, "&")
}

func s3CanonicalHeaders(r *http.Request, signed []string) string {
	lower := map[string]string{}
	for k, vals := range r.Header {
		lower[strings.ToLower(k)] = strings.TrimSpace(strings.Join(vals, ","))
	}
	lower["host"] = r.Host
	var lines []string
	for _, h := range signed {
		lines = append(lines, h+":"+lower[h])
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func s3Signature(secret, dateStamp, region, stringToSign string) string {
	kDate := s3HMAC([]byte("AWS4"+secret), dateStamp)
	kRegion := s3HMAC(kDate, region)
	kService := s3HMAC(kRegion, "s3")
	kSigning := s3HMAC(kService, "aws4_request")
	return hex.EncodeToString(s3HMAC(kSigning, stringToSign))
}

func s3HMAC(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}

func s3Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func s3EncodePath(s string) string {
	return s3Encode(s, true)
}

func s3EncodeQuery(s string) string {
	return s3Encode(s, false)
}

func s3Encode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' || (keepSlash && c == '/') {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
