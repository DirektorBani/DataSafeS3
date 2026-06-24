package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) logActivity(r *http.Request, action, resourceType, resourceName string) {
	info, ok := authFrom(r)
	user := "system"
	if ok {
		user = info.Username
	}
	s.logActivityAs(user, clientIP(r), action, resourceType, resourceName)
}

func (s *Server) logActivityAs(user, ip, action, resourceType, resourceName string) {
	rec := metadata.ActivityRecord{
		User:         user,
		Action:       action,
		ResourceType: resourceType,
		ResourceName: resourceName,
		IPAddress:    ip,
		Timestamp:    time.Now().UTC(),
	}
	_ = s.meta.AppendActivity(rec)
	slog.Info("activity",
		"event", "activity",
		"user", user,
		"action", action,
		"resource_type", resourceType,
		"resource", resourceName,
		"ip_address", ip,
	)
}
