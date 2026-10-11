package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKafkaConfigDisabledWhenBrokersEmpty(t *testing.T) {
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "PLAIN")
	cfg, err := KafkaConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled() {
		t.Fatal("empty brokers must not enable the sink")
	}
}

func TestKafkaConfigPlainRequiresTLS(t *testing.T) {
	t.Setenv("STORAGE_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "PLAIN")
	t.Setenv("STORAGE_KAFKA_SASL_USERNAME", "u")
	t.Setenv("STORAGE_KAFKA_SASL_PASSWORD", "p")
	t.Setenv("STORAGE_KAFKA_TLS", "")
	_, err := KafkaConfigFromEnv()
	if err == nil {
		t.Fatal("PLAIN without TLS must fail startup config")
	}
	t.Setenv("STORAGE_KAFKA_TLS", "true")
	cfg, err := KafkaConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SASLMechanism != "plain" || !cfg.TLS {
		t.Fatalf("%+v", cfg)
	}
}

func TestKafkaConfigPasswordFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORAGE_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("STORAGE_KAFKA_SASL_USERNAME", "user")
	t.Setenv("STORAGE_KAFKA_SASL_PASSWORD", "")
	t.Setenv("STORAGE_KAFKA_SASL_PASSWORD_FILE", path)
	cfg, err := KafkaConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SASLPassword != "s3cret" || cfg.SASLMechanism != "scram-sha-512" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Topic != "datasafe.events" || cfg.ClientID != "datasafe" {
		t.Fatalf("defaults %+v", cfg)
	}
}

func TestNotificationJSONShape(t *testing.T) {
	body, err := NotificationJSON(Event{
		ID: "01EVENT", Type: "ObjectCreated", Bucket: "b", Key: "a/b", VersionID: "v1", TenantID: "ten",
		Payload: map[string]any{"size": float64(12), "etag": "abc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatal(err)
	}
	recs := msg["Records"].([]any)
	rec := recs[0].(map[string]any)
	if rec["eventName"] != "s3:ObjectCreated:Put" {
		t.Fatalf("eventName %v", rec["eventName"])
	}
	s3 := rec["s3"].(map[string]any)
	bucket := s3["bucket"].(map[string]any)
	object := s3["object"].(map[string]any)
	if bucket["name"] != "b" || object["key"] != "a/b" || object["eTag"] != "abc" || object["versionId"] != "v1" {
		t.Fatalf("%s", body)
	}
	if object["size"].(float64) != 12 {
		t.Fatalf("size %v", object["size"])
	}
	ds := msg["datasafe"].(map[string]any)
	if ds["tenant_id"] != "ten" || ds["event_id"] != "01EVENT" || ds["schema_version"].(float64) != 1 {
		t.Fatalf("datasafe %v", ds)
	}
	if KafkaRecordKey("ten", "b", "a/b") != "ten/b/a/b" {
		t.Fatal(KafkaRecordKey("ten", "b", "a/b"))
	}
}
