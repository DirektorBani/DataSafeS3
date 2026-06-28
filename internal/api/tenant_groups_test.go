package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func setupTenantWithGroups(t *testing.T, s *api.Server, adminTok string) (tenantID, tadminTok, memberTok, outsiderTok string) {
	t.Helper()
	tadminTok = createUser(t, s, adminTok, "grpadmin", auth.RoleUser)
	memberTok = createUser(t, s, adminTok, "grpmember", auth.RoleUser)
	outsiderTok = createUser(t, s, adminTok, "grpoutsider", auth.RoleUser)

	tadmin, _ := s.Meta().GetUserByUsername("grpadmin")
	member, _ := s.Meta().GetUserByUsername("grpmember")

	body, _ := json.Marshal(map[string]string{"name": "GroupCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	tenantID = tenantResp.Tenant.ID

	for _, pair := range []struct {
		userID string
		role   string
	}{
		{tadmin.ID, auth.TenantRoleAdmin},
		{member.ID, "member"},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add member %s: %d %s", pair.role, rec.Code, rec.Body.String())
		}
	}
	return tenantID, tadminTok, memberTok, outsiderTok
}

func TestTenantGroupBucketAccess(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, memberTok, outsiderTok := setupTenantWithGroups(t, s, adminTok)

	tadmin, _ := s.Meta().GetUserByUsername("grpadmin")
	member, _ := s.Meta().GetUserByUsername("grpmember")

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "finance-data", TenantID: tenantID, Owner: tadmin.Username, OwnerID: tadmin.ID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "hr-data", TenantID: tenantID, Owner: tadmin.Username, OwnerID: tadmin.ID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	finRec, _ := s.Meta().ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantID}, "finance-data")

	groupBody, _ := json.Marshal(map[string]any{
		"name": "Finance", "access_level": metadata.GroupAccessReadWrite,
	})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create group %d %s", rec.Code, rec.Body.String())
	}
	var groupResp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &groupResp)
	groupID := groupResp.Group.ID

	bucketsBody, _ := json.Marshal(map[string]any{"bucket_keys": []string{finRec.EffectiveStorageKey()}})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/groups/"+groupID+"/buckets", tadminTok, bucketsBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set group buckets %d %s", rec.Code, rec.Body.String())
	}

	assignBody, _ := json.Marshal(map[string]any{"group_ids": []string{groupID}})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/members/"+member.ID+"/groups", tadminTok, assignBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign groups %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	hasFinance, hasHR := false, false
	for _, b := range list.Buckets {
		if b.Name == "finance-data" {
			hasFinance = true
		}
		if b.Name == "hr-data" {
			hasHR = true
		}
	}
	if !hasFinance {
		t.Fatalf("member should see finance bucket via group, got %+v", list.Buckets)
	}
	if hasHR {
		t.Fatalf("member should not see hr bucket without group, got %+v", list.Buckets)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/hr-data/objects", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member access hr bucket want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/finance-data/objects", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("member read finance bucket want 200 got %d", rec.Code)
	}

	putReq := authReq(http.MethodPut, "/api/v1/buckets/finance-data/objects/ok.txt", memberTok, []byte("data"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("member write finance (read_write group) want 200 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", outsiderTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	for _, b := range list.Buckets {
		if b.Name == "finance-data" || b.Name == "hr-data" {
			t.Fatalf("outsider should not see tenant buckets %+v", list.Buckets)
		}
	}
}

func TestTenantGroupReadOnlyAccess(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, memberTok, _ := setupTenantWithGroups(t, s, adminTok)
	member, _ := s.Meta().GetUserByUsername("grpmember")

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "readonly-bkt", TenantID: tenantID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	brec, _ := s.Meta().ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantID}, "readonly-bkt")

	groupBody, _ := json.Marshal(map[string]any{"name": "Viewers", "access_level": metadata.GroupAccessRead})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var groupResp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &groupResp)

	bucketsBody, _ := json.Marshal(map[string]any{"bucket_keys": []string{brec.EffectiveStorageKey()}})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/groups/"+groupResp.Group.ID+"/buckets", tadminTok, bucketsBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	assignBody, _ := json.Marshal(map[string]any{"group_ids": []string{groupResp.Group.ID}})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/members/"+member.ID+"/groups", tadminTok, assignBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/readonly-bkt/objects/nope.txt", memberTok, []byte("x"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only group upload want 403 got %d", rec.Code)
	}
}

func TestTenantGroupCrossTenantIsolation(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantA, tadminATok, _, _ := setupTenantWithGroups(t, s, adminTok)

	body, _ := json.Marshal(map[string]string{"name": "OtherCorp"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantBResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantBResp)
	tenantB := tenantBResp.Tenant.ID

	tadminB := createUser(t, s, adminTok, "grpadmb", auth.RoleUser)
	tadminBUser, _ := s.Meta().GetUserByUsername("grpadmb")
	addBody, _ := json.Marshal(map[string]string{"user_id": tadminBUser.ID, "role": auth.TenantRoleAdmin})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantB+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	groupBody, _ := json.Marshal(map[string]string{"name": "Secret"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantA+"/groups", tadminATok, groupBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var groupResp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &groupResp)

	req = authReq(http.MethodGet, "/api/v1/tenants/"+tenantA+"/groups/"+groupResp.Group.ID, tadminB, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other tenant admin get group want 403 got %d", rec.Code)
	}
}

func TestTenantGroupAdminOnlyManagement(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, memberTok, _ := setupTenantWithGroups(t, s, adminTok)

	req := authReq(http.MethodGet, "/api/v1/tenants/"+tenantID+"/groups", memberTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member list groups want 403 got %d", rec.Code)
	}

	groupBody, _ := json.Marshal(map[string]string{"name": "Ops"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("tenant admin create group %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateTenantUserWithGroups(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, _, _ := setupTenantWithGroups(t, s, adminTok)

	groupBody, _ := json.Marshal(map[string]string{"name": "Onboarding"})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var groupResp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &groupResp)

	createBody, _ := json.Marshal(map[string]any{
		"username": "groupeduser", "password": "pass123", "role": "member",
		"group_ids": []string{groupResp.Group.ID},
	})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/users", tadminTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user with groups %d %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)

	req = authReq(http.MethodGet, "/api/v1/tenants/"+tenantID+"/members", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var membersResp struct {
		Members []struct {
			UserID string `json:"user_id"`
			Groups []struct {
				ID string `json:"id"`
			} `json:"groups"`
		} `json:"members"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &membersResp)
	for _, m := range membersResp.Members {
		if m.UserID == createResp.User.ID {
			if len(m.Groups) != 1 || m.Groups[0].ID != groupResp.Group.ID {
				t.Fatalf("expected group on new user, got %+v", m.Groups)
			}
			return
		}
	}
	t.Fatal("new user not in members list")
}

func TestTenantMemberWithoutGroupsInGroupedTenant(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, memberTok, _ := setupTenantWithGroups(t, s, adminTok)

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "hidden-bkt", TenantID: tenantID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	groupBody, _ := json.Marshal(map[string]string{"name": "Any"})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodGet, "/api/v1/buckets", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	for _, b := range list.Buckets {
		if b.Name == "hidden-bkt" {
			t.Fatalf("member without groups should not see tenant bucket when groups exist")
		}
	}
}

func TestTenantAdminSeesAllBucketsWithGroups(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, _, _ := setupTenantWithGroups(t, s, adminTok)

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "admin-visible", TenantID: tenantID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	groupBody, _ := json.Marshal(map[string]string{"name": "G1"})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodGet, "/api/v1/buckets", tadminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, b := range list.Buckets {
		if b.Name == "admin-visible" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tenant admin should see ungrouped bucket %+v", list.Buckets)
	}
}

func TestTenantGroupGrantUnion(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, memberTok, _ := setupTenantWithGroups(t, s, adminTok)
	member, _ := s.Meta().GetUserByUsername("grpmember")

	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "grant-only", TenantID: tenantID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	brec, _ := s.Meta().ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantID}, "grant-only")

	groupBody, _ := json.Marshal(map[string]string{"name": "Empty"})
	req := authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/groups", tadminTok, groupBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	grantBody, _ := json.Marshal(map[string]any{
		"grants": []map[string]any{{"user_id": member.ID, "can_read": true, "can_write": false}},
	})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantID+"/buckets/grant-only/access", tadminTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put grants %d %s", rec.Code, rec.Body.String())
	}
	_ = brec

	req = authReq(http.MethodGet, "/api/v1/buckets/grant-only/objects", memberTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("member with grant (no group) want 200 got %d", rec.Code)
	}
}

func TestListTenantBucketsForGroupAssignment(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	tenantID, tadminTok, _, _ := setupTenantWithGroups(t, s, adminTok)
	tadmin, _ := s.Meta().GetUserByUsername("grpadmin")

	// Legacy bucket: tenant member owner but empty tenant_id (pre-migration data).
	if err := s.Meta().PutBucket(metadata.BucketRecord{
		Name: "legacy-tenant-bkt", Owner: tadmin.Username, OwnerID: tadmin.ID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req := authReq(http.MethodGet, "/api/v1/tenants/"+tenantID+"/buckets", tadminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tenant buckets %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Buckets []metadata.BucketRecord `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	found := false
	for _, b := range resp.Buckets {
		if b.Name == "legacy-tenant-bkt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tenant admin should see member-owned legacy bucket %+v", resp.Buckets)
	}
}
