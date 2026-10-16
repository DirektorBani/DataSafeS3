package metadata

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestVaultSecretsSealedWithoutFieldEncryption(t *testing.T) {
	st, err := OpenBolt(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	st.SetKMSSealMaterial("server-key")

	const secret = "plain-secret-id-must-not-hit-disk"
	const token = "hvs.plain-token-must-not-hit-disk"
	cfg := SystemConfig{
		TrashRetentionDays: 30,
		KMS: VaultKMSConfig{
			Provider: "vault-transit",
			Addr:     "http://127.0.0.1:8200",
			Auth:     "approle",
			RoleID:   "role",
			SecretID: secret,
			Token:    token,
		},
	}
	if err := st.PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw := readSystemConfigBytes(t, st)
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte(token)) {
		t.Fatalf("plaintext at rest: %s", raw)
	}
	if !bytes.Contains(raw, []byte(kmsSealPrefix)) {
		t.Fatalf("missing seal prefix: %s", raw)
	}
	got, err := st.GetSystemConfig()
	if err != nil || got.KMS.SecretID != secret || got.KMS.Token != token {
		t.Fatalf("roundtrip %+v %v", got.KMS, err)
	}

	legacy := SystemConfig{TrashRetentionDays: 30, KMS: VaultKMSConfig{SecretID: secret, Token: token}}
	plain, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("config")).Put([]byte("system"), plain)
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := st.GetSystemConfig()
	if err != nil || opened.KMS.SecretID != secret {
		t.Fatalf("legacy plaintext open: %+v %v", opened.KMS, err)
	}
	if err := st.PutSystemConfig(opened); err != nil {
		t.Fatal(err)
	}
	raw = readSystemConfigBytes(t, st)
	if bytes.Contains(raw, []byte(secret)) || !bytes.Contains(raw, []byte(kmsSealPrefix)) {
		t.Fatalf("legacy value was not resealed: %s", raw)
	}
}

func readSystemConfigBytes(t *testing.T, st *Store) []byte {
	t.Helper()
	var raw []byte
	err := st.db.View(func(tx *bolt.Tx) error {
		raw = append([]byte(nil), tx.Bucket([]byte("config")).Get([]byte("system"))...)
		return nil
	})
	if err != nil || len(raw) == 0 {
		t.Fatalf("read system: %v %d", err, len(raw))
	}
	return raw
}
