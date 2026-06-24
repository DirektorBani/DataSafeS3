package storage

import (
	"bytes"
	"testing"
)

func TestSSECipherRoundTrip(t *testing.T) {
	c, err := NewSSECipher("test-master-key-32chars-minimum!!")
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello datasafe sse-s3")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, plain) {
		t.Fatal("expected ciphertext differ from plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip mismatch: %q vs %q", dec, plain)
	}
}

func TestSSECipherDisabled(t *testing.T) {
	c, err := NewSSECipher("")
	if err != nil || c != nil {
		t.Fatalf("empty key should disable SSE: c=%v err=%v", c, err)
	}
}
