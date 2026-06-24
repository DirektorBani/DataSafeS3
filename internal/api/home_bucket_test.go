package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestHomeBucketAutoProvision(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "true")
	t.Setenv("STORAGE_HOME_BUCKET_NAME", "files")
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	createBody, _ := json.Marshal(map[string]string{
		"username": "homeuser", "password": "usr123", "role": auth.RoleUser, "email": "h@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user %d %s", rec.Code, rec.Body.String())
	}

	userTok := loginToken(t, s, "homeuser", "usr123")
	req = authReq(http.MethodGet, "/api/v1/buckets", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list buckets %d", rec.Code)
	}
	var resp struct {
		Buckets []struct {
			Name   string `json:"name"`
			Access struct {
				Ownership string `json:"ownership"`
			} `json:"access"`
		} `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Buckets) != 1 || resp.Buckets[0].Name != "files" {
		t.Fatalf("expected home bucket files, got %+v", resp.Buckets)
	}
	if resp.Buckets[0].Access.Ownership != "owned" {
		t.Fatalf("expected ownership owned, got %s", resp.Buckets[0].Access.Ownership)
	}

	// Idempotent: second /me does not create another bucket
	req = authReq(http.MethodGet, "/api/v1/me", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	req = authReq(http.MethodGet, "/api/v1/buckets", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Buckets) != 1 {
		t.Fatalf("expected still 1 bucket, got %d", len(resp.Buckets))
	}
}

func TestOwnerBucketAccessGrants(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createUser := func(username string) string {
		body, _ := json.Marshal(map[string]string{
			"username": username, "password": "usr123", "role": auth.RoleUser, "email": username + "@test.com",
		})
		req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s %d %s", username, rec.Code, rec.Body.String())
		}
		return loginToken(t, s, username, "usr123")
	}

	ownerTok := createUser("bucketowner")
	granteeTok := createUser("grantee")
	outsiderTok := createUser("outsider")

	owner, _ := s.Meta().GetUserByUsername("bucketowner")
	grantee, _ := s.Meta().GetUserByUsername("grantee")

	// Shared tenant so grantee is shareable for personal bucket
	tenantBody, _ := json.Marshal(map[string]string{"name": "ShareCo"})
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

	for _, pair := range []struct{ userID, role string }{
		{owner.ID, auth.TenantRoleMember},
		{grantee.ID, auth.TenantRoleMember},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add member %d %s", rec.Code, rec.Body.String())
		}
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/mystuff", ownerTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	grantBody, _ := json.Marshal(map[string]any{
		"grants": []map[string]any{
			{"user_id": grantee.ID, "can_read": true, "can_write": false},
		},
	})
	req = authReq(http.MethodPut, "/api/v1/buckets/mystuff/access", ownerTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner put grants %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/mystuff/access", outsiderTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider put grants want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets?filter=shared", granteeTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listResp struct {
		Buckets []struct {
			Name   string `json:"name"`
			Access struct {
				Ownership string `json:"ownership"`
				CanWrite  bool   `json:"can_write"`
			} `json:"access"`
		} `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Buckets) != 1 || listResp.Buckets[0].Name != "mystuff" {
		t.Fatalf("grantee shared list want mystuff, got %+v", listResp.Buckets)
	}
	if listResp.Buckets[0].Access.Ownership != "shared" || listResp.Buckets[0].Access.CanWrite {
		t.Fatalf("grantee should have read-only shared access: %+v", listResp.Buckets[0].Access)
	}

	putReq := authReq(http.MethodPut, "/api/v1/buckets/mystuff/objects/nope.txt", granteeTok, []byte("x"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only grantee upload want 403 got %d", rec.Code)
	}
}

func TestPrefixBucketAccessGrants(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createUser := func(username string) string {
		body, _ := json.Marshal(map[string]string{
			"username": username, "password": "usr123", "role": auth.RoleUser, "email": username + "@test.com",
		})
		req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s %d %s", username, rec.Code, rec.Body.String())
		}
		return loginToken(t, s, username, "usr123")
	}

	ownerTok := createUser("prefixowner")
	granteeTok := createUser("prefixgrantee")

	owner, _ := s.Meta().GetUserByUsername("prefixowner")
	grantee, _ := s.Meta().GetUserByUsername("prefixgrantee")

	tenantBody, _ := json.Marshal(map[string]string{"name": "PrefixCo"})
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

	for _, pair := range []struct{ userID, role string }{
		{owner.ID, auth.TenantRoleMember},
		{grantee.ID, auth.TenantRoleMember},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add member %d %s", rec.Code, rec.Body.String())
		}
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/shared-docs", ownerTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	putOwner := authReq(http.MethodPut, "/api/v1/buckets/shared-docs/objects/reports/q1.pdf", ownerTok, []byte("pdf"))
	putOwner.Header.Set("Content-Type", "application/pdf")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putOwner)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("owner upload reports %d", rec.Code)
	}

	putOwner = authReq(http.MethodPut, "/api/v1/buckets/shared-docs/objects/private/secret.txt", ownerTok, []byte("secret"))
	putOwner.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putOwner)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("owner upload private %d", rec.Code)
	}

	grantBody, _ := json.Marshal(map[string]any{
		"grants": []any{},
		"prefix_grants": []map[string]any{
			{"user_id": grantee.ID, "prefix": "reports/", "can_read": true, "can_write": false},
		},
	})
	req = authReq(http.MethodPut, "/api/v1/buckets/shared-docs/access", ownerTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put prefix grants %d %s", rec.Code, rec.Body.String())
	}

	getOK := authReq(http.MethodGet, "/api/v1/buckets/shared-docs/objects/reports/q1.pdf", granteeTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, getOK)
	if rec.Code != http.StatusOK {
		t.Fatalf("grantee download under prefix want 200 got %d", rec.Code)
	}

	getDeny := authReq(http.MethodGet, "/api/v1/buckets/shared-docs/objects/private/secret.txt", granteeTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, getDeny)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("grantee download outside prefix want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/notifications", granteeTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notifications %d", rec.Code)
	}
	var notifResp struct {
		Unread int `json:"unread"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &notifResp)
	if notifResp.Unread < 1 {
		t.Fatalf("expected unread notification for grantee, got %d", notifResp.Unread)
	}
}

func TestPrefixOnlyBucketShowsAsShared(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createUser := func(username string) string {
		body, _ := json.Marshal(map[string]string{
			"username": username, "password": "usr123", "role": auth.RoleUser, "email": username + "@test.com",
		})
		req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return loginToken(t, s, username, "usr123")
	}

	ownerTok := createUser("powner")
	granteeTok := createUser("pgrantee")
	owner, _ := s.Meta().GetUserByUsername("powner")
	grantee, _ := s.Meta().GetUserByUsername("pgrantee")

	tenantBody, _ := json.Marshal(map[string]string{"name": "PrefixOnlyCo"})
	req := authReq(http.MethodPost, "/api/v1/tenants", adminTok, tenantBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)
	for _, pair := range []struct{ userID, role string }{
		{owner.ID, auth.TenantRoleMember},
		{grantee.ID, auth.TenantRoleMember},
	} {
		addBody, _ := json.Marshal(map[string]string{"user_id": pair.userID, "role": pair.role})
		req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members", adminTok, addBody)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/teamdocs", ownerTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	grantBody, _ := json.Marshal(map[string]any{
		"grants": []any{},
		"prefix_grants": []map[string]any{
			{"user_id": grantee.ID, "prefix": "reports/", "can_read": true, "can_write": false},
		},
	})
	req = authReq(http.MethodPut, "/api/v1/buckets/teamdocs/access", ownerTok, grantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodGet, "/api/v1/buckets?filter=shared", granteeTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var listResp struct {
		Buckets []struct {
			Name   string `json:"name"`
			Access struct {
				Ownership      string   `json:"ownership"`
				SharedPrefixes []string `json:"shared_prefixes"`
			} `json:"access"`
		} `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Buckets) != 1 || listResp.Buckets[0].Access.Ownership != "shared" {
		t.Fatalf("prefix-only grantee want shared filter, got %+v", listResp.Buckets)
	}
	if len(listResp.Buckets[0].Access.SharedPrefixes) != 1 || listResp.Buckets[0].Access.SharedPrefixes[0] != "reports/" {
		t.Fatalf("expected shared_prefixes reports/, got %+v", listResp.Buckets[0].Access.SharedPrefixes)
	}
}
