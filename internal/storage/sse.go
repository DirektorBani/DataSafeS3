package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// SSE-S3 subset: AES-256-GCM with master key from config (32 bytes derived via SHA-256).
type SSECipher struct {
	aead cipher.AEAD
}

func NewSSECipher(masterKey string) (*SSECipher, error) {
	if masterKey == "" {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SSECipher{aead: aead}, nil
}

func (c *SSECipher) Encrypt(plaintext []byte) ([]byte, error) {
	if c == nil {
		return plaintext, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *SSECipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if c == nil {
		return ciphertext, nil
	}
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	return c.aead.Open(nil, nonce, ciphertext[c.aead.NonceSize():], nil)
}

func (c *SSECipher) EncryptReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	enc, err := c.Encrypt(data)
	if err != nil {
		return nil, err
	}
	return &bytesReader{b: enc}, nil
}

func (c *SSECipher) DecryptReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	dec, err := c.Decrypt(data)
	if err != nil {
		return nil, err
	}
	return &bytesReader{b: dec}, nil
}

// SSECustomerKeyHeader is the subset header supported for SSE-S3.
const SSECustomerKeyHeader = "x-amz-server-side-encryption"
const SSEAlgorithmAES256 = "AES256"

type bytesReader struct {
	b   []byte
	off int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

// EncodeSSEKeyID returns a non-secret identifier for the master key (for headers/logging).
func EncodeSSEKeyID(masterKey string) string {
	if masterKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(masterKey))
	return base64.StdEncoding.EncodeToString(sum[:8])
}
