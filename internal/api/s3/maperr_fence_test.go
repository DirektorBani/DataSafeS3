package s3

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMissingObjectBytesAreServiceUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/b/k", nil)
	rec := httptest.NewRecorder()
	mapErr(rec, req, ErrObjectUnavailable)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "ServiceUnavailable") {
		t.Fatalf("unavailable %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "NoSuchKey") {
		t.Fatal("missing bytes were reported as NoSuchKey")
	}
}

func TestFencedPutIsServiceUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/b/k", nil)
	rec := httptest.NewRecorder()
	mapErr(rec, req, errors.New("ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)"))
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "not cluster leader") {
		t.Fatalf("fenced %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	reset := httptest.NewRecorder()
	mapErr(reset, req, errors.New("read tcp 10.0.0.1:5432: connection reset by peer"))
	if reset.Code != http.StatusServiceUnavailable || !strings.Contains(reset.Body.String(), "ServiceUnavailable") {
		t.Fatalf("reset %d %s", reset.Code, reset.Body.String())
	}
	other := httptest.NewRecorder()
	mapErr(other, req, errors.New("disk full"))
	if other.Code != http.StatusInternalServerError || strings.Contains(other.Body.String(), "not cluster leader") {
		t.Fatalf("disk %d %s", other.Code, other.Body.String())
	}
}
