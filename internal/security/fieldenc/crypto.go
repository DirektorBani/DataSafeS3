package fieldenc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const wirePrefix = "enc:v1:"

func aadFor(kekID, fieldPath string) []byte {
	return []byte(fmt.Sprintf("v1|%s|%s", kekID, fieldPath))
}

func deriveWrapKey(priv *ecdh.PrivateKey, peerPub []byte) ([]byte, error) {
	curve := ecdh.X25519()
	pub, err := curve.NewPublicKey(peerPub)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(shared)
	return sum[:], nil
}

func aesGCMEncrypt(key, nonce, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nil
}

func aesGCMDecrypt(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func wrapDEK(wrapKey, dek []byte) ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct, err := aesGCMEncrypt(wrapKey, nonce, dek, nil)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 12+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func unwrapDEK(wrapKey, blob []byte) ([]byte, error) {
	if len(blob) < 12 {
		return nil, fmt.Errorf("fieldenc: wrapped dek too short")
	}
	return aesGCMDecrypt(wrapKey, blob[:12], blob[12:], nil)
}

func encryptValue(kekPriv *ecdh.PrivateKey, kekID, fieldPath, plaintext string) (string, error) {
	curve := ecdh.X25519()
	ephPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	ephPub := ephPriv.PublicKey().Bytes()

	wrapKey, err := deriveWrapKey(ephPriv, kekPriv.PublicKey().Bytes())
	if err != nil {
		return "", err
	}

	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", err
	}

	wrapped, err := wrapDEK(wrapKey, dek)
	if err != nil {
		return "", err
	}

	payloadNonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, payloadNonce); err != nil {
		return "", err
	}

	aad := aadFor(kekID, fieldPath)
	ct, err := aesGCMEncrypt(dek, payloadNonce, []byte(plaintext), aad)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%s:%s:%s:%s:%s",
		wirePrefix,
		kekID,
		base64.StdEncoding.EncodeToString(payloadNonce),
		base64.StdEncoding.EncodeToString(ct),
		base64.StdEncoding.EncodeToString(ephPub),
		base64.StdEncoding.EncodeToString(wrapped),
	), nil
}

func decryptValue(privateKeys map[string]*ecdh.PrivateKey, fieldPath, stored string) (string, error) {
	kekID, payloadNonce, ct, ephPub, wrapped, err := parseWire(stored)
	if err != nil {
		return "", err
	}
	priv, ok := privateKeys[kekID]
	if !ok {
		return "", fmt.Errorf("fieldenc: unknown kek_id %q", kekID)
	}

	wrapKey, err := deriveWrapKey(priv, ephPub)
	if err != nil {
		return "", err
	}
	dek, err := unwrapDEK(wrapKey, wrapped)
	if err != nil {
		return "", err
	}

	plain, err := aesGCMDecrypt(dek, payloadNonce, ct, aadFor(kekID, fieldPath))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func parseWire(stored string) (kekID string, payloadNonce, ct, ephPub, wrapped []byte, err error) {
	if !strings.HasPrefix(stored, wirePrefix) {
		return "", nil, nil, nil, nil, errNotEncrypted
	}
	rest := strings.TrimPrefix(stored, wirePrefix)
	parts := strings.SplitN(rest, ":", 5)
	if len(parts) != 5 || parts[0] == "" {
		return "", nil, nil, nil, nil, fmt.Errorf("fieldenc: invalid wire format")
	}
	kekID = parts[0]
	payloadNonce, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	ct, err = base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	ephPub, err = base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	wrapped, err = base64.StdEncoding.DecodeString(parts[4])
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	return kekID, payloadNonce, ct, ephPub, wrapped, nil
}

func kekIDFromWire(stored string) (string, error) {
	id, _, _, _, _, err := parseWire(stored)
	return id, err
}

var errNotEncrypted = fmt.Errorf("fieldenc: not encrypted")
