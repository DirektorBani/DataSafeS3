package migrate

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"

	"github.com/secure-io/sio-go"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

// MinIO admin responses use the madmin stream header:
// salt (32) | AEAD id (1) | nonce | ciphertext.
const (
	iamArgonAES    = 0x00
	iamArgonChaCha = 0x01
	iamPBKDF2AES   = 0x02
	iamPBKDF2Cost  = 8192
)

// DecryptAdmin opens a madmin ciphertext. A body that is valid JSON is returned
// as-is so a server that does not encrypt the admin API still works. The
// ciphertext starts with a random salt, which can itself be '{' or '[', so the
// first byte is not enough to tell those apart.
func DecryptAdmin(password string, data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if json.Valid(trimmed) {
		return trimmed, nil
	}
	if len(data) < 32+1+8 {
		return nil, errors.New("minio admin response is too short")
	}
	salt, id, rest := data[:32], data[32], data[33:]
	var (
		stream *sio.Stream
		err    error
	)
	switch id {
	case iamArgonAES:
		key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
		stream, err = sio.AES_256_GCM.Stream(key)
	case iamArgonChaCha:
		key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
		stream, err = sio.ChaCha20Poly1305.Stream(key)
	case iamPBKDF2AES:
		key := pbkdf2.Key([]byte(password), salt, iamPBKDF2Cost, 32, sha256.New)
		stream, err = sio.AES_256_GCM.Stream(key)
	default:
		return nil, errors.New("minio admin response has an unknown encryption id")
	}
	if err != nil {
		return nil, err
	}
	nonceLen := stream.NonceSize()
	if len(rest) < nonceLen {
		return nil, errors.New("minio admin response is missing a nonce")
	}
	nonce, ct := rest[:nonceLen], rest[nonceLen:]
	return io.ReadAll(stream.DecryptReader(bytes.NewReader(ct), nonce, nil))
}

// EncryptAdmin seals plaintext with PBKDF2 AES-GCM, the FIPS madmin id.
func EncryptAdmin(password string, plain []byte) ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := pbkdf2.Key([]byte(password), salt, iamPBKDF2Cost, 32, sha256.New)
	stream, err := sio.AES_256_GCM.Stream(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, stream.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(salt)
	buf.WriteByte(iamPBKDF2AES)
	buf.Write(nonce)
	w := stream.EncryptWriter(&buf, nonce, nil)
	if _, err := w.Write(plain); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
