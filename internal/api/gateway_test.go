package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGatewaySyncScheduleAPIAndDueRun(t *testing.T) {
	srv := testServer(t)
	tok := loginToken(t, srv, "admin", "admin")
	conn := metadata.GatewayConnection{
		ID: "conn-sched", Name: "down", Endpoint: "http://127.0.0.1:1",
		Region: "us-east-1", AccessKey: "k", SecretKey: "s", PathStyle: true, CreatedAt: time.Now().UTC(),
	}
	if err := srv.Meta().PutGatewayConnection(conn); err != nil {
		t.Fatal(err)
	}

	postRule := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := authReq(http.MethodPost, "/api/v1/gateway/replication", tok, []byte(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := postRule(`{"source_bucket":"b","dest_connection_id":"conn-sched","dest_bucket":"d","sync_schedule":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid schedule: %d %s", rec.Code, rec.Body.String())
	}
	rec := postRule(`{"source_bucket":"b","dest_connection_id":"conn-sched","dest_bucket":"d"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Rule metadata.ReplicationRule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Rule.SyncSchedule != "" {
		t.Fatalf("empty schedule stored as %q", created.Rule.SyncSchedule)
	}

	patch := authReq(http.MethodPatch, "/api/v1/gateway/replication/"+created.Rule.ID, tok, []byte(`{"sync_schedule":"15m"}`))
	prec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(prec, patch)
	if prec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", prec.Code, prec.Body.String())
	}

	srv.RunDueScheduledSyncs(context.Background())
	got, err := srv.Meta().GetReplicationRule(created.Rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSyncStatus != metadata.SyncJobFailed || got.LastSyncError == "" || got.LastSyncAt == nil {
		t.Fatalf("last run: %+v", got)
	}
	jobs, err := srv.Meta().ListSyncJobs(created.Rule.ID, 10)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %+v err=%v", jobs, err)
	}

	recent := time.Now().UTC()
	got.LastSyncAt = &recent
	if err := srv.Meta().PutReplicationRule(got); err != nil {
		t.Fatal(err)
	}
	srv.RunDueScheduledSyncs(context.Background())
	jobs, _ = srv.Meta().ListSyncJobs(created.Rule.ID, 10)
	if len(jobs) != 1 {
		t.Fatalf("interval not elapsed, jobs=%d", len(jobs))
	}

	off := authReq(http.MethodPatch, "/api/v1/gateway/replication/"+created.Rule.ID, tok, []byte(`{"sync_schedule":"off"}`))
	orec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(orec, off)
	if orec.Code != http.StatusOK {
		t.Fatalf("off: %d %s", orec.Code, orec.Body.String())
	}
	srv.RunDueScheduledSyncs(context.Background())
	jobs, _ = srv.Meta().ListSyncJobs(created.Rule.ID, 10)
	if len(jobs) != 1 {
		t.Fatalf("off must not schedule another job, jobs=%d", len(jobs))
	}
}

func TestCreateGatewayConnectionOmitsSecret(t *testing.T) {
	srv := testServer(t)
	tok := loginToken(t, srv, "admin", "admin")
	const secret = "gateway-secret-do-not-echo"
	body := []byte(`{"name":"ext","endpoint":"http://127.0.0.1:9","region":"us-east-1","access_key":"AKIA","secret_key":"` + secret + `","path_style":true}`)
	req := authReq(http.MethodPost, "/api/v1/gateway/connections", tok, body)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "secret_key") {
		t.Fatalf("response echoed the secret: %s", rec.Body.String())
	}
	var resp struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	got, err := srv.Meta().GetGatewayConnection(resp.Connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretKey != secret {
		t.Fatalf("stored secret %q", got.SecretKey)
	}
}
