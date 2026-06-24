package auth

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

type Credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

type Signer struct {
	Region  string
	Service string
	Lookup  func(accessKey string) (Credentials, bool)
}

func NewSigner(region string, lookup func(string) (Credentials, bool)) *Signer {
	return &Signer{
		Region:  region,
		Service: "s3",
		Lookup:  lookup,
	}
}

// Authenticate validates SigV4 header or presigned query auth.
func (s *Signer) Authenticate(r *http.Request) (Credentials, error) {
	q := r.URL.Query()
	if q.Get("X-Amz-Algorithm") != "" {
		return s.verifyPresigned(r)
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return Credentials{}, ErrMissingAuth
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		return Credentials{}, ErrInvalidAuth
	}
	parts := parseAuthHeader(auth[len("AWS4-HMAC-SHA256 "):])
	credPart, ok := parts["Credential"]
	if !ok {
		return Credentials{}, ErrInvalidAuth
	}
	credFields := strings.Split(credPart, "/")
	if len(credFields) != 5 {
		return Credentials{}, ErrInvalidAuth
	}
	accessKey := credFields[0]
	creds, ok := s.Lookup(accessKey)
	if !ok {
		return Credentials{}, ErrInvalidAuth
	}
	signedHeaders := strings.Split(parts["SignedHeaders"], ";")
	canonical := canonicalRequest(r, signedHeaders)
	stringToSign := buildStringToSign(parts["Credential"], parts["SignedHeaders"], canonical, r)
	expected := sign(creds.SecretKey, credFields[1], s.Region, s.Service, stringToSign)
	if !hmac.Equal([]byte(parts["Signature"]), []byte(expected)) {
		return Credentials{}, ErrSignatureMismatch
	}
	if err := validateSessionToken(r, creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func (s *Signer) verifyPresigned(r *http.Request) (Credentials, error) {
	q := r.URL.Query()
	credPart := q.Get("X-Amz-Credential")
	credFields := strings.Split(credPart, "/")
	if len(credFields) != 5 {
		return Credentials{}, ErrInvalidAuth
	}
	accessKey := credFields[0]
	creds, ok := s.Lookup(accessKey)
	if !ok {
		return Credentials{}, ErrInvalidAuth
	}
	if exp := q.Get("X-Amz-Expires"); exp != "" {
		dateStr := q.Get("X-Amz-Date")
		if dateStr == "" {
			return Credentials{}, ErrInvalidAuth
		}
		t, err := time.Parse("20060102T150405Z", dateStr)
		if err != nil {
			return Credentials{}, ErrInvalidAuth
		}
		var secs int64
		fmt.Sscanf(exp, "%d", &secs)
		if time.Since(t) > time.Duration(secs)*time.Second {
			return Credentials{}, ErrExpired
		}
	}
	signedHeaders := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	canonical := canonicalPresignedRequest(r, signedHeaders)
	stringToSign := buildStringToSign(credPart, q.Get("X-Amz-SignedHeaders"), canonical, r)
	expected := sign(creds.SecretKey, credFields[1], s.Region, s.Service, stringToSign)
	if !hmac.Equal([]byte(q.Get("X-Amz-Signature")), []byte(expected)) {
		return Credentials{}, ErrSignatureMismatch
	}
	if err := validateSessionToken(r, creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func validateSessionToken(r *http.Request, creds Credentials) error {
	if creds.SessionToken == "" {
		return nil
	}
	tok := r.Header.Get("X-Amz-Security-Token")
	if tok == "" {
		tok = r.URL.Query().Get("X-Amz-Security-Token")
	}
	if tok != creds.SessionToken {
		return ErrInvalidAuth
	}
	return nil
}

// PresignURL returns a presigned URL for the given method.
func (s *Signer) PresignURL(method, endpoint, bucket, key string, creds Credentials, expires time.Duration) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	path := "/" + bucket
	if key != "" {
		path += "/" + key
	}
	u.Path = path
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credential := fmt.Sprintf("%s/%s/%s/%s/aws4_request", creds.AccessKey, dateStamp, s.Region, s.Service)
	q := u.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", credential)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	u.RawQuery = q.Encode()

	r, _ := http.NewRequest(method, u.String(), nil)
	r.Host = u.Host
	canonical := canonicalPresignedRequest(r, []string{"host"})
	stringToSign := buildStringToSign(credential, "host", canonical, r)
	sig := sign(creds.SecretKey, dateStamp, s.Region, s.Service, stringToSign)
	q.Set("X-Amz-Signature", sig)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func parseAuthHeader(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		i := strings.Index(part, "=")
		if i <= 0 {
			continue
		}
		out[part[:i]] = strings.Trim(part[i+1:], `"`)
	}
	return out
}

func canonicalRequest(r *http.Request, signedHeaders []string) string {
	var b strings.Builder
	b.WriteString(r.Method)
	b.WriteByte('\n')
	b.WriteString(uriEncode(r.URL.EscapedPath()))
	b.WriteByte('\n')
	b.WriteString(canonicalQuery(r.URL.Query()))
	b.WriteByte('\n')
	b.WriteString(canonicalHeaders(r, signedHeaders))
	b.WriteByte('\n')
	b.WriteString(strings.Join(signedHeaders, ";"))
	b.WriteByte('\n')
	b.WriteString(r.Header.Get("X-Amz-Content-Sha256"))
	return b.String()
}

func canonicalPresignedRequest(r *http.Request, signedHeaders []string) string {
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	for _, k := range []string{"X-Amz-Signature"} {
		q.Del(k)
	}
	var b strings.Builder
	b.WriteString(r.Method)
	b.WriteByte('\n')
	b.WriteString(uriEncode(u.EscapedPath()))
	b.WriteByte('\n')
	b.WriteString(canonicalQuery(q))
	b.WriteByte('\n')
	b.WriteString(canonicalHeaders(r, signedHeaders))
	b.WriteByte('\n')
	b.WriteString(strings.Join(signedHeaders, ";"))
	b.WriteByte('\n')
	b.WriteString("UNSIGNED-PAYLOAD")
	return b.String()
}

func canonicalQuery(q url.Values) string {
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
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncodeQuery(k)+"="+uriEncodeQuery(v))
		}
	}
	return strings.Join(parts, "&")
}

func canonicalHeaders(r *http.Request, signed []string) string {
	lower := make(map[string]string)
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

func buildStringToSign(credential, signedHeaders, canonical string, r *http.Request) string {
	date := r.Header.Get("X-Amz-Date")
	if date == "" {
		date = r.URL.Query().Get("X-Amz-Date")
	}
	hash := sha256.Sum256([]byte(canonical))
	scope := strings.SplitN(credential, "/", 2)
	scopeStr := scope[1]
	return strings.Join([]string{
		"AWS4-HMAC-SHA256",
		date,
		scopeStr,
		hex.EncodeToString(hash[:]),
	}, "\n")
}

func sign(secret, dateStamp, region, service, stringToSign string) string {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else if c == '/' {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

func uriEncodeQuery(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}
