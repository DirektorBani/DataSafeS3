package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const activityExportMaxRows = 50_000

func (s *Server) handleExportActivity(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	q := r.URL.Query()
	format := strings.ToLower(strings.TrimSpace(q.Get("format")))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "format must be csv or json"})
		return
	}
	f := metadata.ActivityFilter{
		Period: q.Get("period"),
		User:   q.Get("user"),
		Action: q.Get("action"),
		Bucket: q.Get("bucket"),
		IP:     q.Get("ip"),
		Search: q.Get("search"),
		Offset: 0,
		Limit:  activityExportMaxRows,
	}
	if !auth.CanSeeAllActivity(info.Role) {
		f.LimitUser = info.Username
	}
	result, err := s.meta.ListActivity(f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionActivityExported, "activity", format)

	filename := fmt.Sprintf("activity-%s.%s", time.Now().UTC().Format("20060102T150405Z"), format)
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events":      result.Events,
			"total":       result.Total,
			"truncated":   len(result.Events) >= activityExportMaxRows,
			"exported_at": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "timestamp", "user", "action", "resource_type", "resource_name", "ip_address"})
	for _, e := range result.Events {
		_ = cw.Write([]string{
			e.ID,
			e.Timestamp.UTC().Format(time.RFC3339),
			e.User,
			e.Action,
			e.ResourceType,
			e.ResourceName,
			e.IPAddress,
		})
	}
	cw.Flush()
}

func activityRetentionDaysFromEnv() int {
	v := strings.TrimSpace(os.Getenv("STORAGE_ACTIVITY_RETENTION_DAYS"))
	if v == "" {
		return 90
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 90
	}
	return n
}
