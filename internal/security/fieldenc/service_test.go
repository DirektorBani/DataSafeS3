package fieldenc

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func testKEKPair(t *testing.T, label string) (kekID string, priv *ecdh.PrivateKey) {
	t.Helper()
	curve := ecdh.X25519()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	priv, err := curve.NewPrivateKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	return "kek-test-" + label, priv
}

func TestRoundtrip(t *testing.T) {
	kekID, priv := testKEKPair(t, "roundtrip")
	svc, err := NewForTest(kekID, priv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	fieldPath := PathAccessKeySecretKey
	plain := "super-secret-value"
	enc, err := svc.Encrypt(fieldPath, plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(enc) {
		t.Fatal("expected encrypted wire format")
	}
	got, err := svc.Decrypt(fieldPath, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestAADTamper(t *testing.T) {
	kekID, priv := testKEKPair(t, "aad")
	svc, err := NewForTest(kekID, priv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := svc.Encrypt(PathAccessKeySecretKey, "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Decrypt(PathAccessKeySessionToken, enc)
	if err == nil {
		t.Fatal("expected AAD mismatch error")
	}
}

func TestPlaintextPassthrough(t *testing.T) {
	kekID, priv := testKEKPair(t, "plain")
	svc, err := NewForTest(kekID, priv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	plain := "legacy-plaintext"
	got, err := svc.Decrypt(PathAccessKeySecretKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
	disabled, err := NewForTest(kekID, priv.Bytes(), false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := disabled.Encrypt(PathAccessKeySecretKey, plain)
	if err != nil || out != plain {
		t.Fatalf("disabled encrypt: got %q err %v", out, err)
	}
}

func TestMultiKeyDecrypt(t *testing.T) {
	oldID, oldPriv := testKEKPair(t, "old")
	newID, newPriv := testKEKPair(t, "new")

	oldSvc, err := NewForTest(oldID, oldPriv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := oldSvc.Encrypt(PathGatewaySecretKey, "gw-secret")
	if err != nil {
		t.Fatal(err)
	}

	multi, err := NewForTest(newID, newPriv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	multi.privateKeys[oldID] = oldPriv

	got, err := multi.Decrypt(PathGatewaySecretKey, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gw-secret" {
		t.Fatalf("decrypt with old key: got %q", got)
	}

	rewrapped, changed, err := multi.RewrapIfNeeded(PathGatewaySecretKey, enc)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected rewrap")
	}
	if !IsEncrypted(rewrapped) {
		t.Fatal("expected encrypted output")
	}
	parts := strings.Split(strings.TrimPrefix(rewrapped, wirePrefix), ":")
	if len(parts) < 1 || parts[0] != newID {
		t.Fatalf("rewrapped blob should use new kek_id, got %q", rewrapped)
	}
	dec, err := multi.Decrypt(PathGatewaySecretKey, rewrapped)
	if err != nil || dec != "gw-secret" {
		t.Fatalf("after rewrap: %q err %v", dec, err)
	}
}

func TestFromEnv_disabledByDefault(t *testing.T) {
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ENABLED", "")
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID", "")
	t.Setenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY", "")
	t.Setenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS", "")
	svc, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if svc.Enabled() {
		t.Fatal("expected disabled when STORAGE_FIELD_ENCRYPTION_ENABLED unset")
	}
	out, err := svc.Encrypt(PathAccessKeySecretKey, "plain-secret")
	if err != nil || out != "plain-secret" {
		t.Fatalf("disabled encrypt: got %q err %v", out, err)
	}
}

func TestFromEnv(t *testing.T) {
	kekID, priv := testKEKPair(t, "env")
	b64 := base64.StdEncoding.EncodeToString(priv.Bytes())
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ENABLED", "true")
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID", kekID)
	t.Setenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY", b64)
	svc, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Enabled() || svc.ActiveKEKID() != kekID {
		t.Fatalf("svc: enabled=%v id=%q", svc.Enabled(), svc.ActiveKEKID())
	}
}

func TestInvalidWireFormat(t *testing.T) {
	kekID, priv := testKEKPair(t, "bad")
	svc, err := NewForTest(kekID, priv.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Decrypt("x", "enc:v1:bad")
	if err == nil {
		t.Fatal("expected error for invalid blob")
	}
}
