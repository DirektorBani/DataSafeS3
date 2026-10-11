package api

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func e03Server(t *testing.T) (*Server, auth.Credentials) {
	t.Helper()
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_DEV", "true")
	dir := t.TempDir()
	srv, err := NewServer(Config{
		DataDir: dir, Region: "us-east-1",
		AccessKey: "testkey", SecretKey: "testsecret",
		AdminUser: "admin", AdminPassword: "admin",
		JWTSecret: "test-jwt-secret",
		Metadata:  metadata.Config{Backend: "bolt", DataDir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	cfg, err := srv.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := srv.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return srv, auth.Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
}

func e03Sign(t testing.TB, req *http.Request, creds auth.Credentials) {
	t.Helper()
	if req.Host == "" {
		req.Host = req.URL.Host
	}
	if err := auth.SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
}

func e03Do(t testing.TB, method, url string, body []byte, creds auth.Credentials, headers map[string]string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	e03Sign(t, req, creds)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func e03MustStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: status %d body %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, b)
	}
}

func e03Count(t *testing.T, meta metadata.MetadataStore) int {
	t.Helper()
	st, ok := meta.(events.OutboxStore)
	if !ok {
		t.Fatal("metadata store has no outbox")
	}
	total := 0
	for _, status := range []string{events.StatusPending, events.StatusDelivered, events.StatusDead, events.StatusDropped} {
		n, err := st.CountOutbox(context.Background(), status)
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	return total
}

func e03Due(t *testing.T, meta metadata.MetadataStore) []events.OutboxRecord {
	t.Helper()
	st := meta.(events.OutboxStore)
	rows, err := st.ListDueOutbox(context.Background(), time.Now().UTC().Add(time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestS3PutWithoutSinksWritesNoOutbox(t *testing.T) {
	srv, creds := e03Server(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/plain", nil, creds, nil), http.StatusOK)
	before := e03Count(t, srv.Meta())
	const n = 8
	for i := 0; i < n; i++ {
		e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/plain/obj", []byte("x"), creds, nil), http.StatusOK)
	}
	if got := e03Count(t, srv.Meta()); got != before {
		t.Fatalf("outbox rows %d, want %d after %d PUTs with no sinks", got, before, n)
	}
}

func TestS3PutWebhookAfterReceiverOutage(t *testing.T) {
	var up atomic.Bool
	var hits atomic.Int32
	var gotBody atomic.Value
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !up.Load() {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			_ = conn.Close()
			return
		}
		hits.Add(1)
		gotBody.Store(append([]byte(nil), body...))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	srv, creds := e03Server(t)
	if err := srv.Meta().PutWebhook(metadata.WebhookRecord{
		ID: "wh1", Name: "hook", URL: receiver.URL, Events: []string{"ObjectCreated"}, Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	srv.syncEventSinks()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/box", nil, creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/box/a.txt", []byte("hello"), creds, nil), http.StatusOK)

	rows := e03Due(t, srv.Meta())
	if len(rows) != 1 || rows[0].EventType != metadata.EventObjectCreated {
		t.Fatalf("rows %+v", rows)
	}

	if err := srv.eventDisp.ProcessOnce(context.Background(), time.Now().UTC().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("receiver accepted during outage: %d", hits.Load())
	}
	pending, err := srv.meta.(events.OutboxStore).GetOutbox(context.Background(), rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != events.StatusPending || pending.Attempts["webhook"].Attempts < 1 {
		t.Fatalf("after outage %+v", pending.Attempts)
	}

	_ = srv.eventDisp.Close()
	srv.eventDisp = events.NewDispatcher(srv.meta.(events.OutboxStore), nil, events.Config{
		MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	srv.syncEventSinks()
	if err := srv.eventDisp.ProcessOnce(context.Background(), time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("delivered while receiver still down: %d", hits.Load())
	}

	up.Store(true)
	if err := srv.eventDisp.ProcessOnce(context.Background(), time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
	raw, _ := gotBody.Load().([]byte)
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["event"] != metadata.EventObjectCreated {
		t.Fatalf("envelope %+v", envelope)
	}
	if _, ok := envelope["data"]; !ok {
		t.Fatalf("envelope missing data: %s", raw)
	}
	if _, ok := envelope["timestamp"]; !ok {
		t.Fatalf("envelope missing timestamp: %s", raw)
	}
	got, err := srv.meta.(events.OutboxStore).GetOutbox(context.Background(), rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("status %s %+v", got.Status, got.Attempts)
	}
}

func TestBucketNotificationFilter(t *testing.T) {
	var bodies [][]byte
	var events []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		events = append(events, r.Header.Get("X-DataSafe-Event"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	srv, creds := e03Server(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/pics", nil, creds, nil), http.StatusOK)

	xmlBody := `<NotificationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <TopicConfiguration>
    <Topic>` + receiver.URL + `</Topic>
    <Event>s3:ObjectCreated:*</Event>
    <Filter><S3Key>
      <FilterRule><Name>prefix</Name><Value>images/</Value></FilterRule>
      <FilterRule><Name>suffix</Name><Value>.jpg</Value></FilterRule>
    </S3Key></Filter>
  </TopicConfiguration>
</NotificationConfiguration>`
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/pics?notification", []byte(xmlBody), creds, map[string]string{"Content-Type": "application/xml"}), http.StatusOK)

	get := e03Do(t, http.MethodGet, ts.URL+"/pics?notification", nil, creds, nil)
	raw, _ := io.ReadAll(get.Body)
	get.Body.Close()
	if get.StatusCode != http.StatusOK || !strings.Contains(string(raw), "images/") || !strings.Contains(string(raw), ".jpg") {
		t.Fatalf("get notification %d %s", get.StatusCode, raw)
	}

	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/pics/images/a.jpg", []byte("jpg"), creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/pics/images/a.txt", []byte("txt"), creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/pics/other/a.jpg", []byte("nope"), creds, nil), http.StatusOK)

	rows := e03Due(t, srv.Meta())
	if len(rows) != 1 {
		t.Fatalf("want 1 bucket notification row, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Key != "images/a.jpg" || rows[0].Attempts["bucket-webhook"].Status == "" {
		t.Fatalf("row %+v", rows[0])
	}
	if strings.Contains(string(rows[0].Payload), "webhook") && !strings.Contains(string(rows[0].Payload), receiver.URL) {
		t.Fatalf("payload %s", rows[0].Payload)
	}

	if err := srv.eventDisp.ProcessOnce(context.Background(), time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 1 {
		t.Fatalf("posts %d", len(bodies))
	}
	if strings.Contains(string(bodies[0]), "_webhook_url") {
		t.Fatalf("private url leaked: %s", bodies[0])
	}
	var posted map[string]any
	if err := json.Unmarshal(bodies[0], &posted); err != nil {
		t.Fatal(err)
	}
	if posted["event"] != "put" || posted["key"] != "images/a.jpg" || events[0] != "put" {
		t.Fatalf("posted %+v header %v", posted, events)
	}
	if _, ok := posted["bucket"]; !ok {
		t.Fatalf("posted %+v", posted)
	}
}

func TestWebhook4xxTerminalAnd429Retry(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer bad.Close()
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow", http.StatusTooManyRequests)
	}))
	defer limited.Close()

	meta, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	now := time.Now().UTC()
	if err := meta.PutWebhook(metadata.WebhookRecord{
		ID: "wh400", Name: "bad", URL: bad.URL, Events: []string{"ObjectCreated"}, Enabled: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := meta.PutWebhook(metadata.WebhookRecord{
		ID: "wh429", Name: "slow", URL: limited.URL, Events: []string{"ObjectDeleted"}, Enabled: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	server := &Server{meta: meta}
	server.eventDisp = events.NewDispatcher(meta, nil, events.Config{
		MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	server.syncEventSinks()

	created := server.outboxRecords("ObjectCreated", "b", "k", "", "", map[string]any{"bucket": "b"})
	deleted := server.outboxRecords("ObjectDeleted", "b", "k", "", "", map[string]any{"bucket": "b"})
	if len(created) != 1 || len(deleted) != 1 {
		t.Fatalf("rows %d %d", len(created), len(deleted))
	}
	if _, err := meta.InsertOutbox(context.Background(), created[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := meta.InsertOutbox(context.Background(), deleted[0]); err != nil {
		t.Fatal(err)
	}
	sentAt := time.Now().UTC()
	if err := server.eventDisp.ProcessOnce(context.Background(), sentAt); err != nil {
		t.Fatal(err)
	}

	got400, err := meta.GetOutbox(context.Background(), created[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	att400 := got400.Attempts["webhook"]
	if got400.Status != events.StatusDead || att400.Attempts != 1 || att400.Status != events.StatusDead || !strings.Contains(att400.LastError, "400") {
		t.Fatalf("400 row status %s %+v", got400.Status, att400)
	}
	d400, err := meta.ListWebhookDeliveries("wh400", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(d400) != 1 || d400[0].StatusCode != http.StatusBadRequest || d400[0].Success || d400[0].Attempts != 1 || !strings.Contains(d400[0].Error, "400") {
		t.Fatalf("400 delivery %+v", d400)
	}

	got429, err := meta.GetOutbox(context.Background(), deleted[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	att429 := got429.Attempts["webhook"]
	if got429.Status != events.StatusPending || att429.Attempts != 1 || att429.Status != events.StatusPending {
		t.Fatalf("429 row status %s %+v", got429.Status, att429)
	}
	if att429.NextAttemptAt.Before(sentAt.Add(30*time.Second)) || att429.NextAttemptAt.After(sentAt.Add(5*time.Minute+time.Second)) {
		t.Fatalf("429 next %s", att429.NextAttemptAt)
	}
	if !strings.Contains(att429.LastError, "429") {
		t.Fatalf("429 last_error %s", att429.LastError)
	}
	d429, err := meta.ListWebhookDeliveries("wh429", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(d429) != 1 || d429[0].StatusCode != http.StatusTooManyRequests || d429[0].Attempts != 1 || d429[0].Success {
		t.Fatalf("429 delivery %+v", d429)
	}
}

func TestS3DeleteCopyAndMultipartEmit(t *testing.T) {
	srv, creds := e03Server(t)
	if err := srv.Meta().PutWebhook(metadata.WebhookRecord{
		ID: "wh", Name: "all", URL: "http://127.0.0.1:9/unused",
		Events:    []string{metadata.EventObjectCreated, metadata.EventObjectDeleted, metadata.EventMultipartCompleted},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	srv.syncEventSinks()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/objs", nil, creds, nil), http.StatusOK)

	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/objs/src.txt", []byte("src"), creds, nil), http.StatusOK)
	e03MustStatus(t, e03Do(t, http.MethodPut, ts.URL+"/objs/dst.txt", nil, creds, map[string]string{
		"x-amz-copy-source": "/objs/src.txt",
	}), http.StatusOK)

	init := e03Do(t, http.MethodPost, ts.URL+"/objs/large.bin?uploads", nil, creds, nil)
	initBody, _ := io.ReadAll(init.Body)
	init.Body.Close()
	if init.StatusCode != http.StatusOK {
		t.Fatalf("init %d %s", init.StatusCode, initBody)
	}
	var initResp struct {
		UploadID string `xml:"UploadId"`
	}
	if err := unmarshalXML(initBody, &initResp); err != nil || initResp.UploadID == "" {
		t.Fatalf("upload id: %v %s", err, initBody)
	}
	part := e03Do(t, http.MethodPut, ts.URL+"/objs/large.bin?partNumber=1&uploadId="+initResp.UploadID, []byte("aaa"), creds, nil)
	etag := part.Header.Get("ETag")
	e03MustStatus(t, part, http.StatusOK)
	complete := `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>` + etag + `</ETag></Part></CompleteMultipartUpload>`
	e03MustStatus(t, e03Do(t, http.MethodPost, ts.URL+"/objs/large.bin?uploadId="+initResp.UploadID, []byte(complete), creds, map[string]string{"Content-Type": "application/xml"}), http.StatusOK)

	buckets, err := srv.Meta().ListBuckets()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range buckets {
		if b.Name == "objs" {
			b.Versioning = true
			if err := srv.Meta().UpdateBucket(b); err != nil {
				t.Fatal(err)
			}
		}
	}
	e03MustStatus(t, e03Do(t, http.MethodDelete, ts.URL+"/objs/src.txt", nil, creds, nil), http.StatusNoContent)

	var created, deleted, multipart int
	for _, row := range e03Due(t, srv.Meta()) {
		switch row.EventType {
		case metadata.EventObjectCreated:
			created++
		case metadata.EventObjectDeleted:
			deleted++
			if row.Key != "src.txt" {
				t.Fatalf("delete row key %s", row.Key)
			}
		case metadata.EventMultipartCompleted:
			multipart++
			if row.Key != "large.bin" {
				t.Fatalf("multipart key %s", row.Key)
			}
		default:
			t.Fatalf("unexpected %s", row.EventType)
		}
	}
	if created < 3 || deleted != 1 || multipart != 1 {
		t.Fatalf("created %d deleted %d multipart %d", created, deleted, multipart)
	}
}

func unmarshalXML(b []byte, dest any) error {
	return xml.NewDecoder(bytes.NewReader(b)).Decode(dest)
}
