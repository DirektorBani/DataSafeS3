package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func benchS3Put(b *testing.B, withSink bool) {
	b.Helper()
	b.Setenv("STORAGE_NATS_URL", "")
	b.Setenv("STORAGE_DEV", "true")
	b.Setenv("STORAGE_EVENTS_MAX_PENDING", "0")
	dir := b.TempDir()
	srv, err := NewServer(Config{
		DataDir: dir, Region: "us-east-1",
		AccessKey: "testkey", SecretKey: "testsecret",
		AdminUser: "admin", AdminPassword: "admin",
		JWTSecret: "test-jwt-secret",
		Metadata:  metadata.Config{Backend: "bolt", DataDir: dir},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = srv.Close() })
	cfg, err := srv.Meta().GetSystemConfig()
	if err != nil {
		b.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := srv.Meta().PutSystemConfig(cfg); err != nil {
		b.Fatal(err)
	}
	if withSink {
		hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		b.Cleanup(hook.Close)
		if err := srv.Meta().PutWebhook(metadata.WebhookRecord{
			ID: "wh", Name: "bench", URL: hook.URL, Events: []string{"ObjectCreated"}, Enabled: true,
		}); err != nil {
			b.Fatal(err)
		}
		srv.syncEventSinks()
	}
	ts := httptest.NewServer(srv.Handler())
	b.Cleanup(ts.Close)
	creds := auth.Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	resp := e03Do(b, http.MethodPut, ts.URL+"/bench", nil, creds, nil)
	e03MustStatus(b, resp, http.StatusOK)
	body := []byte("0123456789abcdef")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := e03Do(b, http.MethodPut, ts.URL+"/bench/obj", body, creds, nil)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			b.Fatalf("status %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func BenchmarkS3PutNoSink(b *testing.B)      { benchS3Put(b, false) }
func BenchmarkS3PutWithWebhook(b *testing.B) { benchS3Put(b, true) }
