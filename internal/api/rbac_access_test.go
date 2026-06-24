package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func createUser(t *testing.T, s *api.Server, adminTok, username, role string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username, "password": "pass123", "role": role, "email": username + "@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user %s: %d %s", username, rec.Code, rec.Body.String())
	}
	return loginToken(t, s, username, "pass123")
}

func TestFederationClusterAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "feduser", auth.RoleUser)
	opTok := createUser(t, s, adminTok, "fedop", auth.RoleOperator)

	for _, tok := range []struct {
		name string
		tok  string
	}{
		{"user", userTok},
		{"operator", opTok},
	} {
		req := authReq(http.MethodGet, "/api/v1/federation/clusters", tok.tok, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s federation want 403 got %d", tok.name, rec.Code)
		}
		req = authReq(http.MethodGet, "/api/v1/cluster/status", tok.tok, nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s cluster want 403 got %d", tok.name, rec.Code)
		}
	}

	req := authReq(http.MethodGet, "/api/v1/federation/clusters", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin federation %d %s", rec.Code, rec.Body.String())
	}
	req = authReq(http.MethodGet, "/api/v1/cluster/status", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin cluster %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserCannotSeeOtherUsersBucket(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "usera", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "userb", auth.RoleUser)

	req := authReq(http.MethodPost, "/api/v1/buckets/usera-bucket", userATok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("usera create bucket %d", rec.Code)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/userb-bucket", userBTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("userb create bucket %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userATok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listA struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listA)
	if len(listA.Buckets) != 1 || listA.Buckets[0].Name != "usera-bucket" {
		t.Fatalf("usera should see only own bucket, got %+v", listA.Buckets)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/userb-bucket/objects", userATok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("usera access userb bucket want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listAdmin struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listAdmin)
	if len(listAdmin.Buckets) < 2 {
		t.Fatalf("admin should see all buckets, got %d", len(listAdmin.Buckets))
	}
}

func TestTeamBucketVisibility(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "teama", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "teamb", auth.RoleUser)

	teamID := "team-shared-1"
	_ = s.Meta().PutTeam(metadata.TeamRecord{ID: teamID, Name: "Shared", CreatedAt: time.Now().UTC()})
	userA, _ := s.Meta().GetUserByUsername("teama")
	userB, _ := s.Meta().GetUserByUsername("teamb")
	userA.TeamID = teamID
	userB.TeamID = teamID
	_ = s.Meta().UpdateUser(userA)
	_ = s.Meta().UpdateUser(userB)
	_ = s.Meta().AddUserTeam(userA.ID, teamID)
	_ = s.Meta().AddUserTeam(userB.ID, teamID)

	req := authReq(http.MethodPost, "/api/v1/buckets/team-bucket", userATok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create team bucket %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userBTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, b := range list.Buckets {
		if b.Name == "team-bucket" {
			found = true
		}
	}
	if !found {
		t.Fatalf("teamb should see team bucket via team_id, got %+v", list.Buckets)
	}
}

func TestOperatorSeesAllBuckets(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	opTok := createUser(t, s, adminTok, "bucketop", auth.RoleOperator)
	userTok := createUser(t, s, adminTok, "bucketusr", auth.RoleUser)

	req := authReq(http.MethodPost, "/api/v1/buckets/op-visible", userTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodGet, "/api/v1/buckets", opTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, b := range list.Buckets {
		if b.Name == "op-visible" {
			found = true
		}
	}
	if !found {
		t.Fatal("operator should see all buckets including user-owned")
	}
}

func TestUserCannotSeeAdminBucket(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "qweuser", auth.RoleUser)

	req := authReq(http.MethodPost, "/api/v1/buckets/admin-only-bucket", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create bucket %d %s", rec.Code, rec.Body.String())
	}

	brec, err := s.Meta().GetBucket("admin-only-bucket")
	if err != nil {
		t.Fatal(err)
	}
	adminUser, err := s.Meta().GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if brec.OwnerID != adminUser.ID {
		t.Fatalf("expected owner_id=%s got %q", adminUser.ID, brec.OwnerID)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	for _, b := range list.Buckets {
		if b.Name == "admin-only-bucket" {
			t.Fatalf("regular user should not see admin bucket, got %+v", list.Buckets)
		}
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/admin-only-bucket/objects", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user access admin bucket want 403 got %d", rec.Code)
	}
}

func TestLegacyOwnerOnlyBucketFiltered(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "legacyuser", auth.RoleUser)

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "legacy-admin-bucket", Owner: "admin", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req := authReq(http.MethodGet, "/api/v1/buckets", userTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	for _, b := range list.Buckets {
		if b.Name == "legacy-admin-bucket" {
			t.Fatalf("user should not see legacy admin bucket by owner username, got %+v", list.Buckets)
		}
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, b := range list.Buckets {
		if b.Name == "legacy-admin-bucket" {
			found = true
		}
	}
	if !found {
		t.Fatal("admin should see legacy admin bucket")
	}
}

func TestTenantMemberBucketAccess(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "tenanta", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "tenantb", auth.RoleUser)

	userA, _ := s.Meta().GetUserByUsername("tenanta")
	userB, _ := s.Meta().GetUserByUsername("tenantb")

	tenantBody, _ := json.Marshal(map[string]string{"name": "Corp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, tenantBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID := tenantResp.Tenant.ID

	addBody, _ := json.Marshal(map[string]string{"user_id": userA.ID, "role": "member"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add member %d %s", rec.Code, rec.Body.String())
	}

	addBody, _ = json.Marshal(map[string]string{"user_id": userB.ID, "role": "viewer"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "tenant-shared", TenantID: tenantID, Owner: "admin", OwnerID: "admin-bootstrap",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userATok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listA struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listA)
	found := false
	for _, b := range listA.Buckets {
		if b.Name == "tenant-shared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tenant member should see tenant bucket, got %+v", listA.Buckets)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userBTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listB struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listB)
	found = false
	for _, b := range listB.Buckets {
		if b.Name == "tenant-shared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tenant viewer should see tenant bucket, got %+v", listB.Buckets)
	}

	putReq := authReq(http.MethodPut, "/api/v1/buckets/tenant-shared/objects/from-viewer.txt", userBTok, []byte("nope"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant viewer upload want 403 got %d", rec.Code)
	}

	putReq = authReq(http.MethodPut, "/api/v1/buckets/tenant-shared/objects/from-member.txt", userATok, []byte("ok"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant member upload want 200 got %d %s", rec.Code, rec.Body.String())
	}

	outsiderTok := createUser(t, s, adminTok, "outsider", auth.RoleUser)
	req = authReq(http.MethodGet, "/api/v1/buckets/tenant-shared/objects", outsiderTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider list tenant bucket want 403 got %d", rec.Code)
	}
}

func TestTenantMemberUsageVisibility(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	memberTok := createUser(t, s, adminTok, "usage-member", auth.RoleUser)
	viewerTok := createUser(t, s, adminTok, "usage-viewer", auth.RoleUser)
	outsiderTok := createUser(t, s, adminTok, "usage-outsider", auth.RoleUser)

	member, _ := s.Meta().GetUserByUsername("usage-member")
	viewer, _ := s.Meta().GetUserByUsername("usage-viewer")

	tenantBody, _ := json.Marshal(map[string]string{"name": "UsageCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, tenantBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID := tenantResp.Tenant.ID

	for _, pair := range []struct {
		userID string
		role   string
	}{
		{member.ID, "member"},
		{viewer.ID, "viewer"},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add %s %d %s", pair.role, rec.Code, rec.Body.String())
		}
	}

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "tenant-usage-bucket", TenantID: tenantID, Owner: "admin", OwnerID: "admin-bootstrap",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Meta().PutObject(metadata.ObjectRecord{
		Bucket: "tenant-usage-bucket", Key: "data.bin", Size: 2048, LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.Meta().AddUsageBytes(5000, 3000)

	assertUsage := func(t *testing.T, tok, label string, wantBuckets int, wantObjects int, wantSize int64, wantXfer bool) {
		t.Helper()
		req := authReq(http.MethodGet, "/api/v1/usage", tok, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s usage %d %s", label, rec.Code, rec.Body.String())
		}
		var resp struct {
			Scope struct {
				SystemWide bool `json:"system_wide"`
			} `json:"scope"`
			Summary struct {
				BucketCount   int   `json:"bucket_count"`
				ObjectCount   int   `json:"object_count"`
				TotalSize     int64 `json:"total_size"`
				UploadBytes   int64 `json:"upload_bytes"`
				DownloadBytes int64 `json:"download_bytes"`
			} `json:"summary"`
			Buckets []metadata.BucketUsage `json:"buckets"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Scope.SystemWide {
			t.Fatalf("%s should not have system-wide scope", label)
		}
		if resp.Summary.BucketCount != wantBuckets {
			t.Fatalf("%s bucket_count=%d want %d", label, resp.Summary.BucketCount, wantBuckets)
		}
		if resp.Summary.ObjectCount != wantObjects {
			t.Fatalf("%s object_count=%d want %d", label, resp.Summary.ObjectCount, wantObjects)
		}
		if resp.Summary.TotalSize != wantSize {
			t.Fatalf("%s total_size=%d want %d", label, resp.Summary.TotalSize, wantSize)
		}
		found := false
		for _, b := range resp.Buckets {
			if b.Name == "tenant-usage-bucket" {
				found = true
				if b.ObjectCount != 1 || b.TotalSize != 2048 {
					t.Fatalf("%s tenant bucket stats %+v", label, b)
				}
			}
		}
		if wantBuckets > 0 && !found {
			t.Fatalf("%s should include tenant-usage-bucket in buckets %+v", label, resp.Buckets)
		}
		hasXfer := resp.Summary.UploadBytes > 0 || resp.Summary.DownloadBytes > 0
		if hasXfer != wantXfer {
			t.Fatalf("%s transfer stats: has=%v want=%v (upload=%d download=%d)",
				label, hasXfer, wantXfer, resp.Summary.UploadBytes, resp.Summary.DownloadBytes)
		}
	}

	assertUsage(t, memberTok, "tenant member", 1, 1, 2048, true)
	assertUsage(t, viewerTok, "tenant viewer", 1, 1, 2048, false)
	assertUsage(t, outsiderTok, "outsider", 0, 0, 0, false)

	req = authReq(http.MethodGet, "/api/v1/usage", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var adminResp struct {
		Scope struct {
			SystemWide bool `json:"system_wide"`
		} `json:"scope"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &adminResp)
	if !adminResp.Scope.SystemWide {
		t.Fatal("admin should have system-wide usage scope")
	}
}

func TestS3ListBucketsFilteredByAccessKey(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "s3usera", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "s3userb", auth.RoleUser)

	req := authReq(http.MethodPost, "/api/v1/buckets/s3a-bucket", userATok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	req = authReq(http.MethodPost, "/api/v1/buckets/s3b-bucket", userBTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	keyBody, _ := json.Marshal(map[string]string{"label": "s3"})
	req = authReq(http.MethodPost, "/api/v1/keys", userATok, keyBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var keyResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &keyResp)

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	creds := auth.Credentials{AccessKey: keyResp["access_key"], SecretKey: keyResp["secret_key"]}
	if err := auth.SignRequest(listReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("s3 list buckets status %d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	if !bytes.Contains(body.Bytes(), []byte("s3a-bucket")) {
		t.Fatalf("expected s3a-bucket in listing: %s", body.String())
	}
	if bytes.Contains(body.Bytes(), []byte("s3b-bucket")) {
		t.Fatalf("user a key should not list user b bucket: %s", body.String())
	}
}

func TestTenantScopedDuplicateBucketNames(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "dupusera", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "dupuserb", auth.RoleUser)

	userA, _ := s.Meta().GetUserByUsername("dupusera")
	userB, _ := s.Meta().GetUserByUsername("dupuserb")

	createTenant := func(name string) string {
		body, _ := json.Marshal(map[string]string{"name": name})
		req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var resp struct {
			Tenant struct {
				ID string `json:"id"`
			} `json:"tenant"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp.Tenant.ID
	}
	tenantA := createTenant("DupCorpA")
	tenantB := createTenant("DupCorpB")

	addMember := func(tenantID, userID, role string) {
		body, _ := json.Marshal(map[string]string{"user_id": userID, "role": role})
		req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add member %s: %d %s", role, rec.Code, rec.Body.String())
		}
	}
	addMember(tenantA, userA.ID, "member")
	addMember(tenantB, userB.ID, "member")

	const sharedName = "shared-bucket-name"
	for _, pair := range []struct {
		tok  string
		user string
	}{
		{userATok, "dupusera"},
		{userBTok, "dupuserb"},
	} {
		req := authReq(http.MethodPost, "/api/v1/buckets/"+sharedName, pair.tok, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s create %q: %d %s", pair.user, sharedName, rec.Code, rec.Body.String())
		}
	}

	recA, err := s.Meta().ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantA}, sharedName)
	if err != nil {
		t.Fatal(err)
	}
	recB, err := s.Meta().ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantB}, sharedName)
	if err != nil {
		t.Fatal(err)
	}
	if recA.EffectiveStorageKey() == recB.EffectiveStorageKey() {
		t.Fatalf("expected different storage keys, both %q", recA.EffectiveStorageKey())
	}
	if recA.Name != sharedName || recB.Name != sharedName {
		t.Fatalf("logical names should match: %+v %+v", recA, recB)
	}

	// duplicate within same tenant scope must conflict
	req := authReq(http.MethodPost, "/api/v1/buckets/"+sharedName, userATok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate in tenant want 409 got %d", rec.Code)
	}
}

func TestTenantBucketAccessGrants(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tadminTok := createUser(t, s, adminTok, "grantadmin", auth.RoleUser)
	memberTok := createUser(t, s, adminTok, "grantmember", auth.RoleUser)
	viewerTok := createUser(t, s, adminTok, "grantviewer", auth.RoleUser)

	tadmin, _ := s.Meta().GetUserByUsername("grantadmin")
	member, _ := s.Meta().GetUserByUsername("grantmember")
	viewer, _ := s.Meta().GetUserByUsername("grantviewer")

	body, _ := json.Marshal(map[string]string{"name": "GrantCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID := tenantResp.Tenant.ID

	for _, pair := range []struct {
		userID string
		role   string
	}{
		{tadmin.ID, auth.TenantRoleAdmin},
		{member.ID, "member"},
		{viewer.ID, "viewer"},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add %s %d %s", pair.role, rec.Code, rec.Body.String())
		}
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/grant-bucket", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/grant-bucket/objects", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("member before grants want 200 got %d", rec.Code)
	}

	grantBody, _ := json.Marshal(map[string]any{
		"grants": []map[string]any{
			{"user_id": viewer.ID, "can_read": true, "can_write": false},
		},
	})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/buckets/grant-bucket/access", tadminTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put grants %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/grant-bucket/objects", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member after grants want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/grant-bucket/objects", viewerTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer with read grant want 200 got %d", rec.Code)
	}

	putReq := authReq(http.MethodPut, "/api/v1/buckets/grant-bucket/objects/nope.txt", viewerTok, []byte("x"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer without write grant want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/tenants/"+tenantID+"/buckets/grant-bucket/access", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin list access want 403 got %d", rec.Code)
	}
}

func TestTenantAdminMemberManagement(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tadminTok := createUser(t, s, adminTok, "tadminuser", auth.RoleUser)
	memberTok := createUser(t, s, adminTok, "tmemuser", auth.RoleUser)
	outsiderTok := createUser(t, s, adminTok, "toutsider", auth.RoleUser)

	tadmin, _ := s.Meta().GetUserByUsername("tadminuser")
	member, _ := s.Meta().GetUserByUsername("tmemuser")

	body, _ := json.Marshal(map[string]string{"name": "AdminCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID := tenantResp.Tenant.ID

	addBody, _ := json.Marshal(map[string]string{"user_id": tadmin.ID, "role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap tenant admin %d %s", rec.Code, rec.Body.String())
	}

	// tenant admin lists tenants (only their own)
	req = authReq(http.MethodGet, "/api/v1/tenants", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant admin list tenants %d", rec.Code)
	}
	var tenantsList struct {
		Tenants []struct {
			ID string `json:"id"`
		} `json:"tenants"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantsList)
	if len(tenantsList.Tenants) != 1 || tenantsList.Tenants[0].ID != tenantID {
		t.Fatalf("tenant admin should see only managed tenant: %+v", tenantsList.Tenants)
	}

	// outsider cannot list tenants
	req = authReq(http.MethodGet, "/api/v1/tenants", outsiderTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider list tenants want 403 got %d", rec.Code)
	}

	// tenant admin adds member
	addBody, _ = json.Marshal(map[string]string{"user_id": member.ID, "role": "member"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", tadminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("tenant admin add member %d %s", rec.Code, rec.Body.String())
	}

	// regular member cannot add members
	addBody, _ = json.Marshal(map[string]string{"user_id": outsiderTok, "role": "viewer"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", memberTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member add member want 403 got %d", rec.Code)
	}

	// /me exposes tenant memberships
	req = authReq(http.MethodGet, "/api/v1/me", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me %d", rec.Code)
	}
	var meResp struct {
		IsTenantAdmin    bool `json:"is_tenant_admin"`
		TenantMemberships []struct {
			TenantID string `json:"tenant_id"`
			Role     string `json:"role"`
		} `json:"tenant_memberships"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &meResp)
	if !meResp.IsTenantAdmin {
		t.Fatal("me should report is_tenant_admin")
	}
	if len(meResp.TenantMemberships) != 1 || meResp.TenantMemberships[0].Role != auth.TenantRoleAdmin {
		t.Fatalf("me memberships: %+v", meResp.TenantMemberships)
	}

	// tenant admin can list users for member picker
	req = authReq(http.MethodGet, "/api/v1/users", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant admin list users %d", rec.Code)
	}

	// global admin still sees all tenants
	req = authReq(http.MethodGet, "/api/v1/tenants", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list tenants %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantsList)
	if len(tenantsList.Tenants) < 1 {
		t.Fatal("admin should see tenants")
	}
}

func TestTenantAdminCreateUser(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tadminTok := createUser(t, s, adminTok, "createadmin", auth.RoleUser)
	memberTok := createUser(t, s, adminTok, "createmember", auth.RoleUser)
	outsiderAdminTok := createUser(t, s, adminTok, "otheradmin", auth.RoleUser)

	tadmin, _ := s.Meta().GetUserByUsername("createadmin")

	body, _ := json.Marshal(map[string]string{"name": "CreateCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID := tenantResp.Tenant.ID

	body, _ = json.Marshal(map[string]string{"name": "OtherCorp"})
	req = authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var otherTenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &otherTenantResp)
	otherTenantID := otherTenantResp.Tenant.ID

	addBody, _ := json.Marshal(map[string]string{"user_id": tadmin.ID, "role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap tenant admin %d %s", rec.Code, rec.Body.String())
	}

	otherAdmin, _ := s.Meta().GetUserByUsername("otheradmin")
	addBody, _ = json.Marshal(map[string]string{"user_id": otherAdmin.ID, "role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+otherTenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap other tenant admin %d %s", rec.Code, rec.Body.String())
	}

	createBody, _ := json.Marshal(map[string]string{
		"username": "newtenantuser",
		"password": "pass123",
		"email":    "new@test.com",
		"role":     auth.TenantRoleMember,
	})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/users", tadminTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("tenant admin create user %d %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
		Member struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"member"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	if createResp.User.Username != "newtenantuser" || createResp.User.Role != auth.RoleUser {
		t.Fatalf("unexpected user: %+v", createResp.User)
	}
	if createResp.Member.Role != auth.TenantRoleMember {
		t.Fatalf("unexpected member role: %+v", createResp.Member)
	}

	// tenant admin cannot create user in other tenant
	req = authReq(http.MethodPost, "/api/v1/tenants/"+otherTenantID+"/users", tadminTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other tenant create want 403 got %d", rec.Code)
	}

	// regular member cannot create users
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/users", memberTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member create want 403 got %d", rec.Code)
	}

	// tenant admin cannot assign tenant_admin role
	badRole, _ := json.Marshal(map[string]string{
		"username": "badroleuser",
		"password": "pass123",
		"role":     auth.TenantRoleAdmin,
	})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/users", tadminTok, badRole)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("tenant admin assign tenant_admin want 400 got %d", rec.Code)
	}

	// global admin still creates via POST /users
	globalBody, _ := json.Marshal(map[string]string{
		"username": "globalnew", "password": "pass123", "role": auth.RoleUser,
	})
	req = authReq(http.MethodPost, "/api/v1/users", adminTok, globalBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("global admin create user %d %s", rec.Code, rec.Body.String())
	}

	// global admin can create tenant user with tenant_admin via members endpoint
	globalUserBody, _ := json.Marshal(map[string]string{
		"username": "promoted", "password": "pass123", "role": auth.RoleUser,
	})
	req = authReq(http.MethodPost, "/api/v1/users", adminTok, globalUserBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var promotedResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &promotedResp)
	addBody, _ = json.Marshal(map[string]string{"user_id": promotedResp.ID, "role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("global admin assign tenant_admin %d %s", rec.Code, rec.Body.String())
	}

	// tenant admin cannot promote to tenant_admin via update
	updBody, _ := json.Marshal(map[string]string{"role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/members/"+createResp.User.ID, tadminTok, updBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant admin promote want 403 got %d", rec.Code)
	}

	_ = outsiderAdminTok
}

func TestOwnerUploadsOwnBucketDespiteLegacyNameCollision(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	adminUser, _ := s.Meta().GetUserByUsername("admin")
	userTok := createUser(t, s, adminTok, "collisionowner", auth.RoleUser)
	user, _ := s.Meta().GetUserByUsername("collisionowner")

	const bucketName = "collision-bucket"
	now := time.Now().UTC()
	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: bucketName, Owner: "admin", OwnerID: adminUser.ID, TenantID: metadata.DefaultTenantID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	ownerKey := metadata.MakeStorageKey(metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: user.ID}, bucketName)
	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: bucketName, StorageKey: ownerKey, Owner: user.Username, OwnerID: user.ID,
		TenantID: metadata.DefaultTenantID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"name": "CollisionCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	addBody, _ := json.Marshal(map[string]string{"user_id": user.ID, "role": "member"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add tenant member %d %s", rec.Code, rec.Body.String())
	}

	putReq := authReq(http.MethodPut, "/api/v1/buckets/"+bucketName+"/objects/data.txt", userTok, []byte("mine"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner upload despite legacy name collision want 200 got %d %s", rec.Code, rec.Body.String())
	}

	obj, err := s.Meta().GetObject(ownerKey, "data.txt")
	if err != nil {
		t.Fatalf("object in owner bucket: %v", err)
	}
	if obj.Size != 4 {
		t.Fatalf("unexpected object size %d", obj.Size)
	}
}
