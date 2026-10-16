package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductionSpecHasNoDeprecations(t *testing.T) {
	if len(apiDeprecations) != 0 {
		t.Fatalf("A0 production spec must not deprecate operations, got %d", len(apiDeprecations))
	}
}

func TestDeprecationHeadersFromFixture(t *testing.T) {
	const spec = `
openapi: 3.1.0
paths:
  /widgets/{id}:
    get:
      deprecated: true
      x-since: "1.6.1"
      x-deprecated-at: "1700000000"
      x-sunset: "2027-01-01"
      x-notes-url: "https://example.com/notes"
      x-replacement: "GET /widgets"
  /buckets/{bucket}/objects/{key...}:
    delete:
      deprecated: true
      x-since: "1.6.1"
      x-sunset: "Wed, 21 Oct 2015 07:28:00 GMT"
`
	notices, err := parseDeprecations([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 2 {
		t.Fatalf("notices %d", len(notices))
	}
	n, ok := matchDeprecation(http.MethodGet, "/api/v1/widgets/abc", notices)
	if !ok {
		t.Fatal("widget route did not match")
	}
	rec := httptest.NewRecorder()
	writeDeprecationHeaders(rec, n)
	if rec.Header().Get("Deprecation") != "@1700000000" {
		t.Fatalf("Deprecation %q", rec.Header().Get("Deprecation"))
	}
	if !strings.Contains(rec.Header().Get("Sunset"), "2027") {
		t.Fatalf("Sunset %q", rec.Header().Get("Sunset"))
	}
	if rec.Header().Get("Link") != `<https://example.com/notes>; rel="deprecation"` {
		t.Fatalf("Link %q", rec.Header().Get("Link"))
	}
	if _, ok := matchDeprecation(http.MethodDelete, "/api/v1/buckets/b/objects/a/b.txt", notices); !ok {
		t.Fatal("wildcard object key did not match")
	}
	if _, ok := matchDeprecation(http.MethodGet, "/api/v1/widgets/abc", notices[:0]); ok {
		t.Fatal("empty notice list matched")
	}
}
