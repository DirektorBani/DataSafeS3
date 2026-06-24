package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const setupTestObjectKey = ".datasafe-setup-test"

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initial_setup_completed":      cfg.InitialSetupCompleted,
		"admin_first_login_completed": cfg.AdminFirstLoginCompleted,
		"admin_password_changed":      cfg.AdminPasswordChanged,
		"needs_password_change":       s.adminNeedsPasswordChange(cfg),
		"needs_setup":                !cfg.InitialSetupCompleted,
	})
}

func (s *Server) adminNeedsPasswordChange(cfg metadata.SystemConfig) bool {
	return !cfg.InitialSetupCompleted && !cfg.AdminPasswordChanged
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if cfg.InitialSetupCompleted {
		writeJSON(w, http.StatusOK, map[string]any{
			"initial_setup_completed": true,
			"needs_setup":             false,
		})
		return
	}
	if !cfg.AdminPasswordChanged {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "password_change_required"})
		return
	}
	cfg.InitialSetupCompleted = true
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "setup", "initial setup completed without external s3")
	writeJSON(w, http.StatusOK, map[string]any{
		"initial_setup_completed": true,
		"needs_setup":             false,
	})
}

func (s *Server) handleSetupS3Test(w http.ResponseWriter, r *http.Request) {
	var req metadata.ExternalS3Config
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateExternalS3(req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	ok, msg := s.testExternalS3(req)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "message": msg})
}

func (s *Server) handleSetupS3Save(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !cfg.AdminPasswordChanged {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "password_change_required"})
		return
	}
	var req metadata.ExternalS3Config
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateExternalS3(req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	ok, msg := s.testExternalS3(req)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection test failed", "message": msg})
		return
	}
	cfg.ExternalS3 = req
	cfg.InitialSetupCompleted = true
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = s.provisionGatewayFromExternalS3(req)
	s.logActivity(r, metadata.ActionSettingsChanged, "setup", "external s3 configured")
	writeJSON(w, http.StatusOK, map[string]any{
		"initial_setup_completed": true,
		"needs_setup":             false,
	})
}

func validateExternalS3(cfg metadata.ExternalS3Config) string {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return "endpoint required"
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		return "access_key_id required"
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return "secret_access_key required"
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return "bucket required"
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return "region required"
	}
	return ""
}

func (s *Server) externalS3GatewayConn(cfg metadata.ExternalS3Config) metadata.GatewayConnection {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if cfg.UseSSL && !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://") {
		endpoint = "https://" + endpoint
	}
	if !cfg.UseSSL && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	return metadata.GatewayConnection{
		Endpoint:  endpoint,
		Region:    cfg.Region,
		AccessKey: cfg.AccessKeyID,
		SecretKey: cfg.SecretAccessKey,
		PathStyle: true,
		TLSVerify: cfg.UseSSL,
	}
}

func (s *Server) testExternalS3(cfg metadata.ExternalS3Config) (bool, string) {
	conn := s.externalS3GatewayConn(cfg)
	client, err := s.gatewayS3Client(conn)
	if err != nil {
		return false, err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)})
	if err != nil {
		return false, err.Error()
	}

	body := []byte("ok")
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(cfg.Bucket),
		Key:           aws.String(setupTestObjectKey),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String("text/plain"),
	})
	if err != nil {
		return false, "bucket reachable but write failed: " + err.Error()
	}
	_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(setupTestObjectKey),
	})
	return true, "connected"
}

func (s *Server) provisionGatewayFromExternalS3(cfg metadata.ExternalS3Config) error {
	conns, err := s.meta.ListGatewayConnections()
	if err != nil {
		return err
	}
	conn := s.externalS3GatewayConn(cfg)
	for _, c := range conns {
		if c.Name == "default" || (c.Endpoint == conn.Endpoint && c.AccessKey == conn.AccessKey) {
			return nil
		}
	}
	conn.ID = randomID()
	conn.Name = "default"
	conn.Status = "ok"
	conn.LastCheck = time.Now().UTC()
	conn.CreatedAt = time.Now().UTC()
	return s.meta.PutGatewayConnection(conn)
}

func (s *Server) markAdminFirstLoginIfNeeded(user metadata.UserRecord) {
	if user.Role != metadata.RoleAdministrator {
		return
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || cfg.AdminFirstLoginCompleted {
		return
	}
	cfg.AdminFirstLoginCompleted = true
	_ = s.meta.PutSystemConfig(cfg)
}

func (s *Server) guardSetup(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.meta.GetSystemConfig()
		if err != nil || cfg.InitialSetupCompleted {
			next(w, r)
			return
		}
		info, ok := authFrom(r)
		if ok && info.Role == auth.RoleAdministrator {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "setup_required"})
			return
		}
		next(w, r)
	}
}
