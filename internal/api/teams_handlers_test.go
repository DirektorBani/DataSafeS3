package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestTeamsCRUD(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]string{"name": "Engineering"})
	req := authReq(http.MethodPost, "/api/v1/teams", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create team %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Team metadata.TeamRecord `json:"team"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Team.ID == "" {
		t.Fatal("expected team id")
	}

	req = authReq(http.MethodGet, "/api/v1/teams", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list teams %d", rec.Code)
	}

	upd, _ := json.Marshal(map[string]string{"name": "Eng Updated"})
	req = authReq(http.MethodPut, "/api/v1/teams/"+created.Team.ID, adminTok, upd)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update team %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodDelete, "/api/v1/teams/"+created.Team.ID, adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete team %d", rec.Code)
	}
}

func TestTeamMembersAssign(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "teamember", auth.RoleUser)

	teamID := "team-assign-1"
	_ = s.Meta().PutTeam(metadata.TeamRecord{ID: teamID, Name: "Assign", CreatedAt: time.Now().UTC()})
	user, _ := s.Meta().GetUserByUsername("teamember")

	memBody, _ := json.Marshal(map[string]any{"user_ids": []string{user.ID}})
	req := authReq(http.MethodPut, "/api/v1/teams/"+teamID+"/members", adminTok, memBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put members %d %s", rec.Code, rec.Body.String())
	}

	ids, _ := s.Meta().ListTeamMemberUserIDs(teamID)
	if len(ids) != 1 || ids[0] != user.ID {
		t.Fatalf("expected member %s, got %v", user.ID, ids)
	}

	req = authReq(http.MethodGet, "/api/v1/teams/"+teamID+"/members", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list members %d", rec.Code)
	}

	_ = userTok
}

func TestTeamBucketVisibilityViaAPI(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userATok := createUser(t, s, adminTok, "api-teama", auth.RoleUser)
	userBTok := createUser(t, s, adminTok, "api-teamb", auth.RoleUser)

	teamBody, _ := json.Marshal(map[string]string{"name": "API Team"})
	req := authReq(http.MethodPost, "/api/v1/teams", adminTok, teamBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var teamResp struct {
		Team metadata.TeamRecord `json:"team"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &teamResp)
	teamID := teamResp.Team.ID

	userA, _ := s.Meta().GetUserByUsername("api-teama")
	userB, _ := s.Meta().GetUserByUsername("api-teamb")
	userA.TeamID = teamID
	userB.TeamID = teamID
	_ = s.Meta().UpdateUser(userA)
	_ = s.Meta().UpdateUser(userB)

	memBody, _ := json.Marshal(map[string]any{"user_ids": []string{userA.ID, userB.ID}})
	req = authReq(http.MethodPut, "/api/v1/teams/"+teamID+"/members", adminTok, memBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodPost, "/api/v1/buckets/api-team-bucket", userATok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
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
		if b.Name == "api-team-bucket" {
			found = true
		}
	}
	if !found {
		t.Fatalf("teamb should see team bucket, got %+v", list.Buckets)
	}
}

func TestTeamsAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "noteams", auth.RoleUser)

	req := authReq(http.MethodGet, "/api/v1/teams", userTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user list teams want 403 got %d", rec.Code)
	}
}
