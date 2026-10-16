package main

// Bodies taken from the release tags, not from a guessed status line.
//
// v1.4.1 (7380cb7) internal/api/ha_handlers.go handleHealthz writes status,
// read_only_mode, and object_backend. It does not write version. The default
// backend is fs (internal/storage/object_backend.go, empty STORAGE_OBJECT_BACKEND).
// HA and postgres fields are omitted when those modes are off. GET
// /api/v1/auth/login-options is registered (internal/api/server.go) and returns
// 200. GET /api/v1/events/delivery is not registered, so the mux answers 404.
//
// v1.5.0 (6b181e8) handleHealthz adds version and commit from
// internal/buildinfo (defaults Version "dev", Commit "unknown"; release
// builds replace them with -ldflags). internal/api/admin_test.go
// TestHealthReportsVersionAndCommit requires both to be non-empty.
// GET /api/v1/events/delivery is adminOnly, and requireAuth answers 401
// when the request has no Bearer token.
var (
	healthzV141 = []byte(`{"object_backend":"fs","read_only_mode":false,"status":"ok"}`)
	healthzV150 = []byte(`{"commit":"unknown","object_backend":"fs","read_only_mode":false,"status":"ok","version":"dev"}`)
)

// healthzUnknown keeps the v1.5.0 field set and replaces version with a
// value that is present but not a semver and not "dev".
var healthzUnknown = []byte(`{"commit":"unknown","object_backend":"fs","read_only_mode":false,"status":"ok","version":"banana"}`)

// healthzBlankVersion is the v1.5.0 field set with an empty version.
var healthzBlankVersion = []byte(`{"commit":"unknown","object_backend":"fs","read_only_mode":false,"status":"ok","version":""}`)
