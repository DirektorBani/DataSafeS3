package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SignRequest adds AWS SigV4 Authorization headers to an outbound request.
func SignRequest(r *http.Request, creds Credentials, region, service string, payloadHash string) error {
	if service == "" {
		service = "s3"
	}
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if r.Header.Get("Host") == "" {
		r.Header.Set("Host", r.Host)
	}
	signedHeaders := signedHeaderNames(r)
	canonical := canonicalRequest(r, signedHeaders)
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hashHex(canonical),
	}, "\n")
	sig := sign(creds.SecretKey, dateStamp, region, service, stringToSign)
	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKey, credentialScope, strings.Join(signedHeaders, ";"), sig,
	)
	r.Header.Set("Authorization", authHeader)
	return nil
}

// signedHeaderNames is the AWS set: host, every x-amz-* header, and
// Content-Type and Content-MD5 when the request sends them.
func signedHeaderNames(r *http.Request) []string {
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

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func HashPayload(body []byte) string {
	if body == nil {
		return "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	}
	return hashHex(string(body))
}
