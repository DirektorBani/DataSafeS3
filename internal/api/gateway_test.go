package api_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestReplicationEnqueueOnPut(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	bucket := "repl-src"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	conn := metadata.GatewayConnection{
		ID:        "conn1",
		Name:      "mock",
		Endpoint:  "http://127.0.0.1:9",
		Region:    "us-east-1",
		AccessKey: "k",
		SecretKey: "s",
		PathStyle: true,
		CreatedAt: time.Now().UTC(),
	}
	if err := srv.Meta().PutGatewayConnection(conn); err != nil {
		t.Fatal(err)
	}
	rule := metadata.ReplicationRule{
		ID:             "rule1",
		SourceBucket:   bucket,
		DestConnection: conn.ID,
		DestBucket:     "remote-bucket",
		Enabled:        true,
		CreatedAt:      time.Now().UTC(),
	}
	if err := srv.Meta().PutReplicationRule(rule); err != nil {
		t.Fatal(err)
	}

	body := []byte("hello-replication")
	_, err := srv.Svc().PutObject(ctx, bucket, "test.txt", bytes.NewReader(body), int64(len(body)), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := srv.Meta().ListReplicationTasks(metadata.ReplTaskPending, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected replication task enqueued")
	}
	if tasks[0].Key != "test.txt" || tasks[0].Event != metadata.ReplEventPut {
		t.Fatalf("unexpected task: %+v", tasks[0])
	}
}

func TestReplicationRulesForBucket(t *testing.T) {
	srv := testServer(t)
	rule := metadata.ReplicationRule{
		ID:             "r1",
		SourceBucket:   "b1",
		DestConnection: "c1",
		DestBucket:     "d1",
		Enabled:        true,
		CreatedAt:      time.Now().UTC(),
	}
	if err := srv.Meta().PutReplicationRule(rule); err != nil {
		t.Fatal(err)
	}
	rules, err := srv.Meta().ListReplicationRulesForBucket("b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	disabled := rule
	disabled.ID = "r2"
	disabled.Enabled = false
	_ = srv.Meta().PutReplicationRule(disabled)
	rules, _ = srv.Meta().ListReplicationRulesForBucket("b1")
	if len(rules) != 1 {
		t.Fatalf("disabled rule should be excluded, got %d", len(rules))
	}
}
