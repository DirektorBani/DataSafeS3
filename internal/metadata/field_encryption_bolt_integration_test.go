package metadata

import (
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	bolt "go.etcd.io/bbolt"
)

func boltFieldEncEnv(t *testing.T, enabled bool) *fieldenc.Service {
	t.Helper()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	svc, err := fieldenc.NewForTest("kek-bolt-itest", seed, enabled)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func openBoltWithFieldEnc(t *testing.T, enabled bool) *Store {
	t.Helper()
	s, err := OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.fieldenc = boltFieldEncEnv(t, enabled)
	return s
}

func TestBoltFieldEncryption_PutAccessKey_encryptedAtRest(t *testing.T) {
	s := openBoltWithFieldEnc(t, true)

	ak := "AKIABOLT" + time.Now().Format("150405")
	secret := "bolt-field-enc-secret"
	rec := AccessKeyRecord{
		AccessKey: ak,
		SecretKey: secret,
		Label:     "bolt-fe-test",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.PutAccessKey(rec); err != nil {
		t.Fatal(err)
	}

	var stored AccessKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("access_keys")).Get([]byte(ak))
		if data == nil {
			t.Fatal("access key not in bolt")
		}
		return json.Unmarshal(data, &stored)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.SecretKey, "enc:v1:") {
		t.Fatalf("expected enc:v1 in bolt, got %q", stored.SecretKey)
	}

	got, err := s.GetAccessKey(ak)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretKey != secret {
		t.Fatalf("decrypted secret %q want %q", got.SecretKey, secret)
	}
}

func TestBoltFieldEncryption_PutGatewayConnection_encryptedAtRest(t *testing.T) {
	s := openBoltWithFieldEnc(t, true)

	conn := GatewayConnection{
		ID:        "gw-fe-bolt",
		Name:      "fe-gw",
		Endpoint:  "https://s3.example.com",
		Region:    "us-east-1",
		AccessKey: "GWAKIA",
		SecretKey: "gw-secret-key",
		PathStyle: true,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.PutGatewayConnection(conn); err != nil {
		t.Fatal(err)
	}

	var stored GatewayConnection
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("gateway_connections")).Get([]byte(conn.ID))
		return json.Unmarshal(data, &stored)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.SecretKey, "enc:v1:") {
		t.Fatalf("expected enc:v1 secret_key in bolt, got %q", stored.SecretKey)
	}

	got, err := s.GetGatewayConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretKey != conn.SecretKey || got.AccessKey != conn.AccessKey {
		t.Fatalf("decrypted gateway creds mismatch: %+v", got)
	}
}

func TestBoltFieldEncryption_disabled_plaintext(t *testing.T) {
	s := openBoltWithFieldEnc(t, false)

	ak := "AKIAPLAIN" + time.Now().Format("150405")
	secret := "plaintext-bolt-secret"
	rec := AccessKeyRecord{
		AccessKey: ak,
		SecretKey: secret,
		Label:     "bolt-plain-test",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.PutAccessKey(rec); err != nil {
		t.Fatal(err)
	}

	var stored AccessKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("access_keys")).Get([]byte(ak))
		return json.Unmarshal(data, &stored)
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.SecretKey != secret {
		t.Fatalf("expected plaintext in bolt, got %q", stored.SecretKey)
	}
	got, err := s.GetAccessKey(ak)
	if err != nil || got.SecretKey != secret {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}
