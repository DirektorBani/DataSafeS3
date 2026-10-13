package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func putShareWebhook(t *testing.T, s interface{ Meta() metadata.MetadataStore }) {
	t.Helper()
	if err := s.Meta().PutWebhook(metadata.WebhookRecord{
		ID: "wh-share", Name: "all", URL: "http://127.0.0.1:9",
		Events: []string{"*"}, Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func countOutboxType(t *testing.T, s interface{ Meta() metadata.MetadataStore }, eventType string) int {
	t.Helper()
	st, ok := s.Meta().(events.OutboxStore)
	if !ok {
		t.Fatal("metadata store has no outbox")
	}
	rows, err := st.ListDueOutbox(context.Background(), time.Now().Add(time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row.EventType != eventType {
			continue
		}
		n++
		if _, ok := row.Attempts["webhook"]; !ok {
			t.Fatalf("%s outbox row has no webhook attempt: %+v", eventType, row.Attempts)
		}
		var payload map[string]any
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["link_id"] == "" {
			t.Fatalf("%s payload missing link_id: %s", eventType, row.Payload)
		}
	}
	return n
}

func countActivity(t *testing.T, s interface{ Meta() metadata.MetadataStore }, action string) int {
	t.Helper()
	res, err := s.Meta().ListActivity(metadata.ActivityFilter{Action: action, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return res.Total
}

func TestShareLimitReachedEmitsOnce(t *testing.T) {
	s := testServer(t)
	putShareWebhook(t, s)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/limbox", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	put := authReq(http.MethodPut, "/api/v1/buckets/limbox/objects/note.txt", tok, []byte("hello"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)

	body, _ := json.Marshal(map[string]any{"key": "note.txt", "expires_in_sec": 3600, "max_downloads": 1})
	req = authReq(http.MethodPost, "/api/v1/buckets/limbox/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	for i, want := range []int{http.StatusOK, http.StatusForbidden, http.StatusForbidden} {
		pub := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, pub)
		if rec.Code != want {
			t.Fatalf("download %d status %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if n := countOutboxType(t, s, "ShareLimitReached"); n != 1 {
		t.Fatalf("ShareLimitReached outbox rows %d, want 1", n)
	}
	if n := countActivity(t, s, metadata.ActionShareLimitReached); n != 1 {
		t.Fatalf("share.limit_reached activity %d, want 1", n)
	}
}

func TestShareExpiredEmitsOnce(t *testing.T) {
	s := testServer(t)
	putShareWebhook(t, s)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/expbox", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	put := authReq(http.MethodPut, "/api/v1/buckets/expbox/objects/note.txt", tok, []byte("hello"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)

	body, _ := json.Marshal(map[string]any{"key": "note.txt", "expires_in_sec": 3600})
	req = authReq(http.MethodPost, "/api/v1/buckets/expbox/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	link, err := s.Meta().GetSharedLink(created.Share.ID)
	if err != nil {
		t.Fatal(err)
	}
	link.Token = created.Share.Token
	past := time.Now().Add(-time.Minute)
	link.ExpiresAt = &past
	if err := s.Meta().PutSharedLink(link); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		pub := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, pub)
		if rec.Code != http.StatusGone {
			t.Fatalf("download %d status %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if n := countOutboxType(t, s, "ShareExpired"); n != 1 {
		t.Fatalf("ShareExpired outbox rows %d, want 1", n)
	}
	if n := countActivity(t, s, metadata.ActionShareExpired); n != 1 {
		t.Fatalf("share.expired activity %d, want 1", n)
	}
}

func TestShareBlockedReachesOutbox(t *testing.T) {
	s := testServer(t)
	putShareWebhook(t, s)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/blkbox", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	put := authReq(http.MethodPut, "/api/v1/buckets/blkbox/objects/note.txt", tok, []byte("hello"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)
	body, _ := json.Marshal(map[string]any{"key": "note.txt", "expires_in_sec": 3600})
	req = authReq(http.MethodPost, "/api/v1/buckets/blkbox/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	off, _ := json.Marshal(map[string]any{"share_links_allowed": false})
	req = authReq(http.MethodPut, "/api/v1/buckets/blkbox/sharing", tok, off)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable %d %s", rec.Code, rec.Body.String())
	}
	for i := 0; i < 2; i++ {
		pub := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, pub)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("blocked %d status %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if n := countOutboxType(t, s, "ShareBlocked"); n != 2 {
		t.Fatalf("ShareBlocked outbox rows %d, want 2", n)
	}
	if n := countActivity(t, s, metadata.ActionShareBlocked); n != 2 {
		t.Fatalf("share.blocked activity %d, want 2", n)
	}
}
