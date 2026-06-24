package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
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
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
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
