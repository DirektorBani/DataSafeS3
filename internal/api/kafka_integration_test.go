package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// TestKafkaBrokerIntegration needs a real broker.
// CI sets KAFKA_INTEGRATION=1 and STORAGE_KAFKA_BROKERS (default localhost:9092).
// go test ./... without that variable skips this test.
func TestKafkaBrokerIntegration(t *testing.T) {
	if os.Getenv("KAFKA_INTEGRATION") != "1" {
		t.Skip("KAFKA_INTEGRATION=1 is not set")
	}
	brokers := strings.TrimSpace(os.Getenv("STORAGE_KAFKA_BROKERS"))
	if brokers == "" {
		brokers = "localhost:9092"
	}
	topic := "datasafe.it." + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000"), ".", "")
	// Apache Kafka 3.9 in this job does not host a missing topic. The idempotent
	// producer then fails with UNKNOWN_TOPIC_OR_PARTITION and nothing is consumed.
	ensureKafkaTopic(t, brokers, topic)
	srv, creds := e03Server(t)
	// e03Server clears brokers so the no-sink tests stay quiet. Register after that.
	t.Setenv("STORAGE_KAFKA_BROKERS", brokers)
	t.Setenv("STORAGE_KAFKA_TOPIC", topic)
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "")
	if err := srv.syncKafkaSink(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv.StartBackground(ctx)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)

	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/kafkabucket", nil, creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/kafkabucket/warmup/obj", []byte("warmup"), creds, nil), http.StatusOK)

	got := collectKafka(t, cl, 1, 45*time.Second)
	assertKafkaRecord(t, got[0], "kafkabucket", "warmup/obj")

	container := kafkaContainer(t)
	if err := exec.Command("docker", "stop", container).Run(); err != nil {
		t.Fatalf("docker stop %s: %v", container, err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "start", container).Run() })

	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/kafkabucket/ordered/file", []byte("v1"), creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodDelete, ts.URL+"/kafkabucket/ordered/file", nil, creds, nil), http.StatusNoContent)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/kafkabucket/other/file", []byte("z"), creds, nil), http.StatusOK)

	deadline := time.Now().Add(20 * time.Second)
	var pending int
	for time.Now().Before(deadline) {
		pending, err = srv.outboxStore().CountOutbox(context.Background(), events.StatusPending)
		if err != nil {
			t.Fatal(err)
		}
		if pending >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if pending < 3 {
		t.Fatalf("broker down: pending %d, want at least 3", pending)
	}

	if err := exec.Command("docker", "start", container).Run(); err != nil {
		t.Fatalf("docker start %s: %v", container, err)
	}
	waitTCP(t, brokers, 60*time.Second)

	deadline = time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		pending, err = srv.outboxStore().CountOutbox(context.Background(), events.StatusPending)
		if err != nil {
			t.Fatal(err)
		}
		if pending == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pending != 0 {
		t.Fatalf("after broker restart, pending %d", pending)
	}

	all := append([]*kgo.Record{}, got...)
	all = append(all, collectKafka(t, cl, 3, 45*time.Second)...)
	if len(all) != 4 {
		t.Fatalf("messages %d, want 4 (no loss)", len(all))
	}
	var ordered []string
	sawOther := false
	ids := map[string]int{}
	for _, rec := range all {
		var msg map[string]any
		if err := json.Unmarshal(rec.Value, &msg); err != nil {
			t.Fatal(err)
		}
		ds := msg["datasafe"].(map[string]any)
		id, _ := ds["event_id"].(string)
		ids[id]++
		records := msg["Records"].([]any)
		object := records[0].(map[string]any)["s3"].(map[string]any)["object"].(map[string]any)
		key, _ := object["key"].(string)
		name := records[0].(map[string]any)["eventName"].(string)
		if key == "ordered/file" {
			ordered = append(ordered, name)
		}
		if key == "other/file" {
			sawOther = true
		}
	}
	if len(ids) != 4 {
		t.Fatalf("distinct event_id %d, want 4 (duplicate delivery or loss)", len(ids))
	}
	if !sawOther {
		t.Fatal("other key was not delivered")
	}
	if len(ordered) != 2 || ordered[0] != "s3:ObjectCreated:Put" || ordered[1] != "s3:ObjectRemoved:Delete" {
		t.Fatalf("per-key order %v", ordered)
	}
}

func ensureKafkaTopic(t *testing.T, brokers, topic string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = []kmsg.CreateTopicsRequestTopic{{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}}
	resp, err := req.RequestWith(ctx, cl)
	if err != nil {
		t.Fatalf("create topic %s: %v", topic, err)
	}
	for _, top := range resp.Topics {
		if err := kerr.ErrorForCode(top.ErrorCode); err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
			t.Fatalf("create topic %s: %v", topic, err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		mreq := kmsg.NewPtrMetadataRequest()
		mreq.Topics = []kmsg.MetadataRequestTopic{{Topic: kmsg.StringPtr(topic)}}
		mresp, err := mreq.RequestWith(ctx, cl)
		if err == nil {
			for _, top := range mresp.Topics {
				if kerr.ErrorForCode(top.ErrorCode) != nil || len(top.Partitions) == 0 {
					continue
				}
				if top.Partitions[0].Leader >= 0 {
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("topic %s has no leader", topic)
}

func assertKafkaRecord(t *testing.T, rec *kgo.Record, bucket, key string) {
	t.Helper()
	var msg map[string]any
	if err := json.Unmarshal(rec.Value, &msg); err != nil {
		t.Fatal(err)
	}
	records, _ := msg["Records"].([]any)
	if len(records) != 1 {
		t.Fatalf("Records %#v", msg["Records"])
	}
	row := records[0].(map[string]any)
	if row["eventName"] != "s3:ObjectCreated:Put" {
		t.Fatalf("eventName %v", row["eventName"])
	}
	s3 := row["s3"].(map[string]any)
	if s3["bucket"].(map[string]any)["name"] != bucket {
		t.Fatalf("bucket %#v", s3["bucket"])
	}
	obj := s3["object"].(map[string]any)
	if obj["key"] != key {
		t.Fatalf("key %v", obj["key"])
	}
	if _, ok := obj["size"]; !ok {
		t.Fatal("missing size")
	}
	if _, ok := obj["eTag"]; !ok {
		t.Fatal("missing eTag")
	}
	if _, ok := obj["versionId"]; !ok {
		t.Fatal("missing versionId")
	}
	ds := msg["datasafe"].(map[string]any)
	if ds["schema_version"].(float64) != 1 {
		t.Fatalf("schema %#v", ds)
	}
	tenant, _ := ds["tenant_id"].(string)
	wantKey := tenant + "/" + bucket + "/" + key
	if string(rec.Key) != wantKey {
		t.Fatalf("record key %q want %q", rec.Key, wantKey)
	}
	if ds["event_id"] == "" {
		t.Fatal("empty event_id")
	}
}

func collectKafka(t *testing.T, cl *kgo.Client, n int, wait time.Duration) []*kgo.Record {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	var out []*kgo.Record
	for len(out) < n {
		fetches := cl.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 && ctx.Err() != nil && len(out) < n {
			t.Fatalf("consume: %v", errs)
		}
		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			out = append(out, p.Records...)
		})
		if ctx.Err() != nil {
			break
		}
	}
	if len(out) < n {
		t.Fatalf("got %d messages, want %d", len(out), n)
	}
	return out[:n]
}

func kafkaContainer(t *testing.T) string {
	t.Helper()
	if id := strings.TrimSpace(os.Getenv("KAFKA_CONTAINER")); id != "" {
		return id
	}
	out, err := exec.Command("docker", "ps", "--format", "{{.ID}}\t{{.Image}}\t{{.Names}}").Output()
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(strings.ToLower(line), "kafka") {
			fields := strings.Split(line, "\t")
			if len(fields) > 0 && fields[0] != "" {
				return fields[0]
			}
		}
	}
	t.Fatal("no running kafka container; set KAFKA_CONTAINER")
	return ""
}

func waitTCP(t *testing.T, brokers string, wait time.Duration) {
	t.Helper()
	addr := strings.TrimSpace(strings.Split(brokers, ",")[0])
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		cl, err := kgo.NewClient(kgo.SeedBrokers(addr))
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = cl.Ping(ctx)
			cancel()
			cl.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("broker %s did not accept connections", addr)
}
