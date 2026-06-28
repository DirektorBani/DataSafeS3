package observability

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("STORAGE_DEV", "true")
	os.Exit(m.Run())
}

func TestLokiSinkPushTimestampFormat(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("path = %q", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	sink := &lokiSink{cfg: metadata.LogSinkEndpoint{Address: srv.URL}}
	if err := sink.Emit(map[string]any{"msg": "test"}); err != nil {
		t.Fatal(err)
	}
	ts := extractLokiTimestamp(t, gotBody)
	if strings.Contains(ts, "T") || strings.Contains(ts, "-") {
		t.Fatalf("expected unix nanoseconds, got %q", ts)
	}
}

func TestElasticSinkPostToIndex(t *testing.T) {
	var path string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sink := &elasticSink{cfg: metadata.LogSinkEndpoint{Address: srv.URL, Index: "my-logs"}}
	if err := sink.Emit(map[string]any{"msg": "elastic-test"}); err != nil {
		t.Fatal(err)
	}
	if path != "/my-logs/_doc" {
		t.Fatalf("path = %q", path)
	}
	if got["msg"] != "elastic-test" {
		t.Fatalf("body = %#v", got)
	}
}

func TestElasticSinkApiKeyAuth(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sink := &elasticSink{cfg: metadata.LogSinkEndpoint{Address: srv.URL, Index: "logs", Token: "abc123"}}
	if err := sink.Emit(map[string]any{"msg": "elastic"}); err != nil {
		t.Fatal(err)
	}
	if auth != "ApiKey abc123" {
		t.Fatalf("auth = %q", auth)
	}
}

func TestElasticSinkBasicAuth(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sink := &elasticSink{cfg: metadata.LogSinkEndpoint{Address: srv.URL, Index: "logs", Username: "elastic", Password: "ElasticTest123!"}}
	if err := sink.Emit(map[string]any{"msg": "elastic-basic"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("auth = %q", auth)
	}
}

func TestWebhookSinkBearerAuth(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := &webhookLogSink{cfg: metadata.LogSinkEndpoint{Address: srv.URL, Token: "secret-token"}}
	if err := sink.Emit(map[string]any{"msg": "webhook"}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer secret-token" {
		t.Fatalf("auth = %q", auth)
	}
}

func TestSinkManagerReconfigureAndEmit(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := &SinkManager{}
	m.Reconfigure(metadata.LoggingConfig{
		Webhook: metadata.LogSinkEndpoint{Enabled: true, Address: srv.URL},
	})
	m.Emit(map[string]any{"msg": "fanout"})
	time.Sleep(50 * time.Millisecond)
	if count != 1 {
		t.Fatalf("webhook deliveries = %d", count)
	}

	m.Reconfigure(metadata.LoggingConfig{})
	m.Emit(map[string]any{"msg": "disabled"})
	time.Sleep(50 * time.Millisecond)
	if count != 1 {
		t.Fatalf("expected no delivery after disable, count = %d", count)
	}
}

func extractLokiTimestamp(t *testing.T, body map[string]any) string {
	t.Helper()
	streams, ok := body["streams"].([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("streams = %#v", body["streams"])
	}
	stream, ok := streams[0].(map[string]any)
	if !ok {
		t.Fatalf("stream = %#v", streams[0])
	}
	values, ok := stream["values"].([]any)
	if !ok || len(values) == 0 {
		t.Fatalf("values = %#v", stream["values"])
	}
	pair, ok := values[0].([]any)
	if !ok || len(pair) < 1 {
		t.Fatalf("pair = %#v", values[0])
	}
	ts, _ := pair[0].(string)
	return ts
}
