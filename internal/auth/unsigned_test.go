package auth

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testSigner() *Signer {
	return NewSigner("us-east-1", func(key string) (Credentials, bool) {
		if key != "testkey" {
			return Credentials{}, false
		}
		return Credentials{AccessKey: "testkey", SecretKey: "testsecret"}, true
	})
}

func TestUnsignedMetadataRejected(t *testing.T) {
	signer := testSigner()
	creds := Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	req, err := http.NewRequest(http.MethodPut, "http://s3.example/bucket/key", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "s3.example"
	req.Header.Set("x-amz-meta-color", "blue")
	if err := SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	if _, err := signer.Authenticate(req); err != nil {
		t.Fatalf("signed metadata: %v", err)
	}
	req.Header.Set("x-amz-meta-evil", "1")
	_, err = signer.Authenticate(req)
	if !errors.Is(err, ErrUnsignedHeaders) {
		t.Fatalf("got %v", err)
	}
}

func TestUnsignedContentTypeAllowed(t *testing.T) {
	signer := testSigner()
	creds := Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	req, err := http.NewRequest(http.MethodPut, "http://s3.example/bucket/key", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "s3.example"
	if err := SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	if _, err := signer.Authenticate(req); err != nil {
		t.Fatal(err)
	}
}

func TestPresignedContentTypeAllowed(t *testing.T) {
	signer := testSigner()
	creds := Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	raw, err := signer.PresignURL(http.MethodPut, "http://s3.example", "bucket", "key", creds, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, raw, strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "s3.example"
	req.Header.Set("Content-Type", "text/plain")
	if _, err := signer.Authenticate(req); err != nil {
		t.Fatal(err)
	}
}

func TestPresignedExtraHeaderRejected(t *testing.T) {
	signer := testSigner()
	creds := Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	raw, err := signer.PresignURL(http.MethodPut, "http://s3.example", "bucket", "key", creds, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, raw, strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "s3.example"
	req.Header.Set("x-amz-meta-evil", "1")
	_, err = signer.Authenticate(req)
	if !errors.Is(err, ErrUnsignedHeaders) {
		t.Fatalf("got %v", err)
	}
}
