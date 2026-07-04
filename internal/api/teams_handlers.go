package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.meta.ListTeams()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if teams == nil {
		teams = []metadata.TeamRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"teams": teams})
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	rec := metadata.TeamRecord{
		ID:        randomID(),
		Name:      req.Name,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutTeam(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "team", rec.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"team": rec})
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetTeam(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"team": rec})
}

func (s *Server) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetTeam(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	rec.Name = req.Name
	if err := s.meta.UpdateTeam(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"team": rec})
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteTeam(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "team", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	if _, err := s.meta.GetTeam(teamID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	userIDs, err := s.meta.ListTeamMemberUserIDs(teamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if userIDs == nil {
		userIDs = []string{}
	}
	type member struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	var out []member
	for _, uid := range userIDs {
		u, err := s.meta.GetUser(uid)
		if err != nil {
			continue
		}
		out = append(out, member{UserID: u.ID, Username: u.Username, Email: u.Email})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (s *Server) handlePutTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	if _, err := s.meta.GetTeam(teamID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	want := map[string]struct{}{}
	for _, uid := range req.UserIDs {
		if uid == "" {
			continue
		}
		if _, err := s.meta.GetUser(uid); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown user_id: " + uid})
			return
		}
		want[uid] = struct{}{}
	}
	current, err := s.meta.ListTeamMemberUserIDs(teamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for _, uid := range current {
		if _, ok := want[uid]; !ok {
			_ = s.meta.RemoveUserTeam(uid, teamID)
		}
	}
	for uid := range want {
		_ = s.meta.AddUserTeam(uid, teamID)
	}
	s.handleListTeamMembers(w, r)
}
