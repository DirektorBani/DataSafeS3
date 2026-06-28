package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/openapi"
	"github.com/DirektorBani/datasafe/internal/metadata"
	_ "github.com/DirektorBani/datasafe/internal/metadata/postgres"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
)

type Config struct {
	DataDir       string
	Region        string
	AccessKey     string
	SecretKey     string
	AdminUser     string
	AdminPassword string
	JWTSecret     string
	Metadata      metadata.Config
	SSEMasterKey  string
	ReadOnly      bool
}

type Server struct {
	cfg               Config
	meta              metadata.MetadataStore
	backend           *storage.FSBackend
	svc               *s3.Service
	s3                *s3.Handler
	jwt               *auth.JWTManager
	oidcSessions      *auth.OIDCSessionStore
	webauthnSessions  *webauthnSessionStore
	mux               *http.ServeMux
	cluster           *clusterMonitor
	eventSinks        []EventSink
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = "datasafe"
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "datasafesecret"
	}
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = "admin"
	}
	if !cfg.ReadOnly {
		cfg.ReadOnly = readOnlyFromEnv()
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	metaCfg := cfg.Metadata
	if metaCfg.DataDir == "" {
		metaCfg.DataDir = cfg.DataDir
	}
	if metaCfg.Backend == "" {
		metaCfg = metadata.ConfigFromEnv(cfg.DataDir)
	}
	meta, err := metadata.Open(metaCfg)
	if err != nil {
		return nil, err
	}
	backend, err := storage.NewFSBackend(filepath.Join(cfg.DataDir, "objects"))
	if err != nil {
		meta.Close()
		return nil, err
	}
	lookup := func(accessKey string) (auth.Credentials, bool) {
		if accessKey == cfg.AccessKey {
			return auth.Credentials{AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey}, true
		}
		rec, err := meta.GetAccessKey(accessKey)
		if err != nil {
			return auth.Credentials{}, false
		}
		if rec.ExpiresAt != nil && time.Now().UTC().After(*rec.ExpiresAt) {
			return auth.Credentials{}, false
		}
		return auth.Credentials{
			AccessKey:    rec.AccessKey,
			SecretKey:    rec.SecretKey,
			SessionToken: rec.SessionToken,
		}, true
	}
	signer := auth.NewSigner(cfg.Region, lookup)
	_ = meta.PutAccessKey(metadata.AccessKeyRecord{
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		Label:     "bootstrap",
		CreatedAt: time.Now().UTC(),
	})
	svc := s3.NewService(backend, meta, signer, cfg.Region, cfg.AccessKey)
	svc.FederationPeers = func() ([]metadata.FederationCluster, error) {
		return meta.ListFederationClusters()
	}
	if cfg.SSEMasterKey != "" {
		if cipher, err := storage.NewSSECipher(cfg.SSEMasterKey); err == nil {
			svc.SetSSE(cipher)
		}
	}
	s := &Server{
		cfg:              cfg,
		meta:             meta,
		backend:          backend,
		svc:              svc,
		s3:               s3.NewHandler(svc),
		jwt:              auth.NewJWTManager(cfg.JWTSecret, 24*time.Hour),
		oidcSessions:     auth.NewOIDCSessionStore(),
		webauthnSessions: newWebAuthnSessionStore(),
		mux:              http.NewServeMux(),
	}
	if err := s.seedAdminUser(); err != nil {
		meta.Close()
		return nil, err
	}
	if err := meta.EnsureDefaultTenant(); err != nil {
		meta.Close()
		return nil, err
	}
	sysCfg, _ := meta.GetSystemConfig()
	sysCfg = s.mergeEnvConfig(sysCfg)
	sysCfg = s.ensureSetupFlagsForExistingInstall(sysCfg)
	_ = meta.PutSystemConfig(sysCfg)
	observability.SetHostDataDir(cfg.DataDir)
	observability.GlobalSinkManager().Reconfigure(sysCfg.Logging)
	observability.SetStorageStats(s.storageStats)
	observability.SetExtendedStats(s.extendedStats)
	s.wireReplicationHooks()
	s.wireEventSinks()
	s.cluster = newClusterMonitor(s.meta)
	s.routes()
	return s, nil
}

func (s *Server) Close() error {
	return s.meta.Close()
}

// Meta exposes the metadata store for integration tests.
func (s *Server) Meta() metadata.MetadataStore {
	return s.meta
}

// Svc exposes the S3 service for integration tests.
func (s *Server) Svc() *s3.Service {
	return s.svc
}

func (s *Server) StartBackground(ctx context.Context) {
	go s.svc.RunLifecycle(ctx, time.Minute)
	go s.runScheduledDeleteWorker(ctx)
	go s.runTrashPurgeWorker(ctx)
	go s.runReplicationWorker(ctx)
	go s.runReplicationFullSyncWorker(ctx)
	go s.runLDAPSyncWorker(ctx)
	if s.cluster != nil {
		go s.cluster.Run(ctx)
	}
}

func (s *Server) runTrashPurgeWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg, err := s.meta.GetSystemConfig()
			if err != nil || !cfg.SoftDeleteEnabled {
				continue
			}
			purged := s.svc.PruneTrashOnce(ctx, cfg.TrashRetentionDays)
			for _, tr := range purged {
				s.logActivityAs("system", "", metadata.ActionTrashPurged, "object", tr.OriginalBucket+"/"+tr.OriginalKey)
			}
		}
	}
}

func (s *Server) runScheduledDeleteWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted := s.svc.PruneScheduledDeletesOnce(ctx)
			for _, obj := range deleted {
				if obj.Size > 0 {
					_ = s.meta.AddUsageBytes(0, obj.Size)
				}
				s.logActivityAs("system", "", metadata.ActionObjectDeleted, "object", obj.Bucket+"/"+obj.Key)
			}
		}
	}
}

func (s *Server) storageStats() (int, int64) {
	buckets, err := s.meta.ListBuckets()
	if err != nil {
		return 0, 0
	}
	bytes, err := s.meta.TotalObjectBytes()
	if err != nil {
		return len(buckets), 0
	}
	return len(buckets), bytes
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		path := r.URL.Path
		if path == "/healthz" || strings.HasPrefix(path, "/api/") || path == "/metrics" {
			if s.readOnlyGuard(w, r) {
				return
			}
			s.mux.ServeHTTP(w, r)
			return
		}
		if s.readOnlyGuard(w, r) {
			return
		}
		s.s3.ServeHTTP(w, r)
	})
}

func setCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Amz-Date, X-Amz-Content-Sha256")
}

func (s *Server) seedAdminUser() error {
	n, err := s.meta.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		return err
	}
	return s.meta.PutUser(metadata.UserRecord{
		ID:           "admin-bootstrap",
		Username:     s.cfg.AdminUser,
		Email:        s.cfg.AdminUser + "@localhost",
		PasswordHash: hash,
		Role:         metadata.RoleAdministrator,
		Status:       metadata.StatusActive,
		TenantID:     metadata.DefaultTenantID,
		CreatedAt:    time.Now().UTC(),
	})
}

// ensureSetupFlagsForExistingInstall skips the wizard on upgrades where admins
// have already logged in before initial_setup_completed existed in SystemConfig.
func (s *Server) ensureSetupFlagsForExistingInstall(cfg metadata.SystemConfig) metadata.SystemConfig {
	if cfg.InitialSetupCompleted {
		return cfg
	}
	users, err := s.meta.ListUsers()
	if err != nil {
		return cfg
	}
	for _, u := range users {
		if u.LastLogin != nil {
			cfg.AdminFirstLoginCompleted = true
			cfg.AdminPasswordChanged = true
			cfg.InitialSetupCompleted = true
			return cfg
		}
	}
	return cfg
}

func (s *Server) routes() {
	mux := s.mux
	openapi.Register(mux)
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/v1/admin/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/v1/mfa/login", s.handleMFALogin)
	mux.HandleFunc("POST /api/v1/auth/login/mfa", s.handleMFALogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/config", s.handleOIDCPublicConfig)
	mux.HandleFunc("GET /api/v1/auth/oidc/login", s.handleOIDCLogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.handleOIDCCallback)
	mux.HandleFunc("POST /api/v1/auth/oidc/password-login", s.handleOIDCPasswordLogin)

	mux.HandleFunc("GET /api/v1/setup/status", s.handleSetupStatus)
	setupAdmin := s.requireAdmin
	authn := s.requireAuth
	adminOnly := func(next http.HandlerFunc) http.HandlerFunc {
		return s.requireAdmin(s.guardSetup(next))
	}
	allRoles := func(next http.HandlerFunc) http.HandlerFunc {
		return s.requireRole(auth.RoleAdministrator, auth.RoleOperator, auth.RoleUser)(s.guardSetup(next))
	}

	mux.HandleFunc("POST /api/v1/setup/s3/test", setupAdmin(s.handleSetupS3Test))
	mux.HandleFunc("POST /api/v1/setup/s3/save", setupAdmin(s.handleSetupS3Save))
	mux.HandleFunc("POST /api/v1/setup/complete", setupAdmin(s.handleSetupComplete))

	mux.HandleFunc("GET /api/v1/me", authn(s.handleMe))
	mux.HandleFunc("PATCH /api/v1/me/locale", authn(s.handleUpdateLocale))
	mux.HandleFunc("POST /api/v1/me/password", authn(s.handleChangePassword))
	mux.HandleFunc("POST /api/v1/admin/logout", authn(s.handleLogout))

	mux.HandleFunc("GET /api/v1/buckets", allRoles(s.handleListBucketsJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/access", allRoles(s.handleListBucketAccessByBucket))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/access", allRoles(s.handlePutBucketAccessByBucket))
	mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/access/{user_id}", allRoles(s.handleDeleteBucketAccessByBucket))
	mux.HandleFunc("GET /api/v1/shareable-users", allRoles(s.handleShareableUsers))
	mux.HandleFunc("GET /api/v1/notifications", allRoles(s.handleListNotifications))
	mux.HandleFunc("POST /api/v1/notifications/{id}/read", allRoles(s.handleMarkNotificationRead))
	mux.HandleFunc("POST /api/v1/notifications/read-all", allRoles(s.handleMarkAllNotificationsRead))
	mux.HandleFunc("GET /api/v1/recent", allRoles(s.handleListRecent))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}", allRoles(s.handleCreateBucketJSON))
	mux.HandleFunc("DELETE /api/v1/buckets/{bucket}", allRoles(s.handleDeleteBucketJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/objects", allRoles(s.handleListObjectsJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/versions", allRoles(s.handleListObjectVersionsJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/settings", allRoles(s.handleGetBucketSettingsJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/object-meta", allRoles(s.handleObjectMetadataJSON))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/folders", allRoles(s.handleCreateFolderJSON))
	mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/folders", allRoles(s.handleDeleteFolderJSON))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/bulk-delete", allRoles(s.handleBulkDeleteObjectsJSON))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/object-actions", allRoles(s.handleObjectActionsJSON))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/objects/{key...}", allRoles(s.handleDownloadObjectJSON))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/objects/{key...}", allRoles(s.handleUploadObjectJSON))
	mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/objects/{key...}", allRoles(s.handleDeleteObjectJSON))
	mux.HandleFunc("POST /api/v1/presign", allRoles(s.handlePresign))
	mux.HandleFunc("GET /api/v1/keys", allRoles(s.handleListKeys))
	mux.HandleFunc("POST /api/v1/keys", allRoles(s.handleCreateKey))
	mux.HandleFunc("DELETE /api/v1/keys/{accessKey}", allRoles(s.handleDeleteKey))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/policy", adminOnly(s.handleGetPolicy))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/policy", adminOnly(s.handlePutPolicy))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/lifecycle", allRoles(s.handleGetLifecycle))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/lifecycle", allRoles(s.handlePutLifecycle))

	mux.HandleFunc("GET /api/v1/activity", adminOnly(s.handleListActivity))
	mux.HandleFunc("GET /api/v1/usage", allRoles(s.handleUsage))

	mux.HandleFunc("GET /api/v1/users", allRoles(s.handleListUsers))
	mux.HandleFunc("POST /api/v1/users", adminOnly(s.handleCreateUser))
	mux.HandleFunc("PUT /api/v1/users/{id}", adminOnly(s.handleUpdateUser))
	mux.HandleFunc("DELETE /api/v1/users/{id}", adminOnly(s.handleDeleteUser))
	mux.HandleFunc("POST /api/v1/users/{id}/reset-password", adminOnly(s.handleResetPassword))

	mux.HandleFunc("GET /api/v1/settings/buckets", adminOnly(s.handleListBucketSettings))
	mux.HandleFunc("PUT /api/v1/settings/buckets/{name}", adminOnly(s.handleUpdateBucketSettings))

	mux.HandleFunc("GET /api/v1/settings/system", adminOnly(s.handleGetSystemConfig))
	mux.HandleFunc("PUT /api/v1/settings/system", adminOnly(s.handlePutSystemConfig))

	mux.HandleFunc("GET /api/v1/trash", allRoles(s.handleListTrash))
	mux.HandleFunc("POST /api/v1/trash/{id}/restore", allRoles(s.handleRestoreTrash))
	mux.HandleFunc("DELETE /api/v1/trash/{id}", allRoles(s.handlePurgeTrash))

	mux.HandleFunc("GET /api/v1/tokens", allRoles(s.handleListAPITokens))
	mux.HandleFunc("POST /api/v1/tokens", allRoles(s.handleCreateAPIToken))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", allRoles(s.handleDeleteAPIToken))

	mux.HandleFunc("GET /api/v1/webhooks", adminOnly(s.handleListWebhooks))
	mux.HandleFunc("POST /api/v1/webhooks", adminOnly(s.handleCreateWebhook))
	mux.HandleFunc("PUT /api/v1/webhooks/{id}", adminOnly(s.handleUpdateWebhook))
	mux.HandleFunc("DELETE /api/v1/webhooks/{id}", adminOnly(s.handleDeleteWebhook))
	mux.HandleFunc("GET /api/v1/webhooks/templates", adminOnly(s.handleWebhookTemplates))
	mux.HandleFunc("POST /api/v1/hooks/test", adminOnly(s.handleHooksTest))
	mux.HandleFunc("GET /api/v1/webhooks/{id}/deliveries", adminOnly(s.handleListWebhookDeliveries))
	mux.HandleFunc("POST /api/v1/webhooks/{id}/deliveries/{deliveryId}/retry", adminOnly(s.handleRetryWebhookDelivery))

	mux.HandleFunc("GET /api/v1/search", allRoles(s.handleSearch))
	mux.HandleFunc("GET /api/v1/favorites", allRoles(s.handleListFavorites))
	mux.HandleFunc("POST /api/v1/favorites", allRoles(s.handleCreateFavorite))
	mux.HandleFunc("DELETE /api/v1/favorites/{id}", allRoles(s.handleDeleteFavorite))

	mux.HandleFunc("GET /api/v1/buckets/{bucket}/tags", allRoles(s.handleGetBucketTags))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/tags", allRoles(s.handlePutBucketTags))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/object-tags", allRoles(s.handleGetObjectTags))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/object-tags", allRoles(s.handlePutObjectTags))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/object-meta", allRoles(s.handlePutObjectMeta))

	mux.HandleFunc("POST /api/v1/buckets/{bucket}/multipart", allRoles(s.handleInitiateMultipart))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/multipart/{uploadId}/parts/{partNumber}", allRoles(s.handleUploadMultipartPart))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/multipart/{uploadId}/complete", allRoles(s.handleCompleteMultipart))
	mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/multipart/{uploadId}", allRoles(s.handleAbortMultipart))

	// Enterprise features (blocks 16-25)
	mux.HandleFunc("POST /api/v1/settings/ldap/test", adminOnly(s.handleLDAPTest))
	mux.HandleFunc("POST /api/v1/settings/ldap/sync", adminOnly(s.handleLDAPSync))
	mux.HandleFunc("GET /api/v1/settings/security-status", adminOnly(s.handleSecurityStatus))
	mux.HandleFunc("POST /api/v1/mfa/enroll", authn(s.handleMFAEnroll))
	mux.HandleFunc("POST /api/v1/mfa/verify-enroll", authn(s.handleMFAVerifyEnroll))
	mux.HandleFunc("POST /api/v1/mfa/verify", authn(s.handleMFAVerifyEnroll))
	mux.HandleFunc("POST /api/v1/mfa/disable", authn(s.handleMFADisable))
	mux.HandleFunc("POST /api/v1/me/mfa/webauthn/register/begin", authn(s.handleWebAuthnRegisterBegin))
	mux.HandleFunc("POST /api/v1/me/mfa/webauthn/register/finish", authn(s.handleWebAuthnRegisterFinish))
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/begin", s.handleWebAuthnLoginBegin)
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/finish", s.handleWebAuthnLoginFinish)
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/legal-hold", allRoles(s.handleSetLegalHold))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/shares", allRoles(s.handleListSharedLinks))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/shares", allRoles(s.handleCreateSharedLink))
	mux.HandleFunc("DELETE /api/v1/shares/{id}", allRoles(s.handleRevokeSharedLink))
	mux.HandleFunc("GET /api/v1/public/share/{token}", s.handlePublicShareInfo)
	mux.HandleFunc("GET /api/v1/public/share/{token}/download", s.handlePublicShareDownload)

	mux.HandleFunc("GET /api/v1/tenants/{id}/buckets", allRoles(s.handleListTenantBuckets))
	mux.HandleFunc("GET /api/v1/tenants/{id}/groups", allRoles(s.handleListTenantGroups))
	mux.HandleFunc("POST /api/v1/tenants/{id}/groups", allRoles(s.handleCreateTenantGroup))
	mux.HandleFunc("GET /api/v1/tenants/{id}/groups/{group_id}", allRoles(s.handleGetTenantGroup))
	mux.HandleFunc("PUT /api/v1/tenants/{id}/groups/{group_id}", allRoles(s.handleUpdateTenantGroup))
	mux.HandleFunc("DELETE /api/v1/tenants/{id}/groups/{group_id}", allRoles(s.handleDeleteTenantGroup))
	mux.HandleFunc("PUT /api/v1/tenants/{id}/groups/{group_id}/buckets", allRoles(s.handlePutTenantGroupBuckets))
	mux.HandleFunc("PUT /api/v1/tenants/{id}/members/{user_id}/groups", allRoles(s.handlePutMemberGroups))
	mux.HandleFunc("GET /api/v1/tenants/{id}/members", allRoles(s.handleListTenantMembers))
	mux.HandleFunc("POST /api/v1/tenants/{id}/users", allRoles(s.handleCreateTenantUser))
	mux.HandleFunc("POST /api/v1/tenants/{id}/members", allRoles(s.handleAddTenantMember))
	mux.HandleFunc("PUT /api/v1/tenants/{id}/members/{userId}", allRoles(s.handleUpdateTenantMember))
	mux.HandleFunc("DELETE /api/v1/tenants/{id}/members/{userId}", allRoles(s.handleRemoveTenantMember))
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/buckets/{bucket}/access", allRoles(s.handleListBucketAccess))
	mux.HandleFunc("PUT /api/v1/tenants/{tenant}/buckets/{bucket}/access", allRoles(s.handlePutBucketAccess))
	mux.HandleFunc("DELETE /api/v1/tenants/{tenant}/buckets/{bucket}/access/{user_id}", allRoles(s.handleDeleteBucketAccess))

	mux.HandleFunc("GET /api/v1/tenants", allRoles(s.handleListTenants))
	mux.HandleFunc("POST /api/v1/tenants", adminOnly(s.handleCreateTenant))
	mux.HandleFunc("DELETE /api/v1/tenants/{id}", adminOnly(s.handleDeleteTenant))
	mux.HandleFunc("GET /api/v1/gateway/connections", adminOnly(s.handleListGatewayConnections))
	mux.HandleFunc("POST /api/v1/gateway/connections", adminOnly(s.handleCreateGatewayConnection))
	mux.HandleFunc("DELETE /api/v1/gateway/connections/{id}", adminOnly(s.handleDeleteGatewayConnection))
	mux.HandleFunc("POST /api/v1/gateway/connections/{id}/test", adminOnly(s.handleTestGatewayConnection))
	mux.HandleFunc("GET /api/v1/gateway/replication", adminOnly(s.handleListReplicationRules))
	mux.HandleFunc("POST /api/v1/gateway/replication", adminOnly(s.handleCreateReplicationRule))
	mux.HandleFunc("DELETE /api/v1/gateway/replication/{id}", adminOnly(s.handleDeleteReplicationRule))
	mux.HandleFunc("POST /api/v1/gateway/replication/{id}/sync", adminOnly(s.handleTriggerSyncJob))
	mux.HandleFunc("GET /api/v1/gateway/sync-jobs", adminOnly(s.handleListSyncJobs))
	mux.HandleFunc("GET /api/v1/gateway/health", adminOnly(s.handleGatewayHealth))
	mux.HandleFunc("GET /api/v1/gateway/replication/queue", adminOnly(s.handleListReplicationQueue))
	mux.HandleFunc("POST /api/v1/gateway/replication/retry-failed", adminOnly(s.handleRetryFailedReplication))
	mux.HandleFunc("POST /api/v1/gateway/replication/clear-errors", adminOnly(s.handleClearReplicationErrors))
	mux.HandleFunc("GET /api/v1/federation/clusters", adminOnly(s.handleListFederationClusters))
	mux.HandleFunc("POST /api/v1/federation/clusters", adminOnly(s.handleCreateFederationCluster))
	mux.HandleFunc("DELETE /api/v1/federation/clusters/{id}", adminOnly(s.handleDeleteFederationCluster))
	mux.HandleFunc("POST /api/v1/federation/clusters/{id}/test", adminOnly(s.handleFederationTestConnectivity))
	mux.HandleFunc("GET /api/v1/cluster/status", adminOnly(s.handleClusterStatus))
	mux.HandleFunc("POST /api/v1/sts/assume-role", allRoles(s.handleAssumeRole))
	mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/transition-storage-class", allRoles(s.handleTransitionStorageClass))
	mux.HandleFunc("PUT /api/v1/buckets/{bucket}/object-retention", allRoles(s.handlePutObjectRetention))
	mux.HandleFunc("GET /api/v1/buckets/{bucket}/object-retention", allRoles(s.handleGetObjectRetention))

	mux.Handle("GET /metrics", observability.MetricsHandler(s.storageStats))
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		MFACode  string `json:"mfa_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	user, err := s.meta.GetUserByUsername(req.Username)
	if err != nil {
		cfg, _ := s.meta.GetSystemConfig()
		if cfg.LDAP.Enabled {
			user, err = s.tryLDAPLogin(req.Username, req.Password)
		}
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
			return
		}
	} else {
		if user.Status != metadata.StatusActive {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "account suspended"})
			return
		}
		if !auth.CheckPassword(user.PasswordHash, req.Password) {
			cfg, _ := s.meta.GetSystemConfig()
			if cfg.LDAP.Enabled {
				user, err = s.tryLDAPLogin(req.Username, req.Password)
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
					return
				}
			} else {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
				return
			}
		}
	}
	if user.Status != metadata.StatusActive {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "account suspended"})
		return
	}
	sysCfg, _ := s.meta.GetSystemConfig()
	needMFA := user.MFAEnabled
	adminNeedsMFASetup := false
	if sysCfg.MFA.RequireAdminMFA && user.Role == metadata.RoleAdministrator {
		hasMFA := user.MFAEnabled || user.WebAuthnCredentials != ""
		if !hasMFA {
			adminNeedsMFASetup = true
		} else {
			needMFA = true
		}
	}
	if needMFA && user.MFAEnabled && !adminNeedsMFASetup {
		mfaToken, err := s.jwt.IssueMFAToken(user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "mfa token issue failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    mfaToken,
		})
		return
	}
	now := time.Now().UTC()
	user.LastLogin = &now
	_ = s.meta.UpdateUser(user)
	s.markAdminFirstLoginIfNeeded(user)
	tokenInfo := auth.TokenInfo{
		Username: user.Username,
		UserID:   user.ID,
		Role:     user.Role,
	}
	s.ensureHomeBucket(tokenInfo)
	token, err := s.jwt.Issue(tokenInfo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token issue failed"})
		return
	}
	s.logActivityAs(user.Username, clientIP(r), metadata.ActionLogin, "session", user.Username)
	resp := map[string]any{
		"token":       token,
		"expires_in":  86400,
		"username":    user.Username,
		"role":        user.Role,
		"user_id":     user.ID,
		"mfa_enabled": user.MFAEnabled,
	}
	if adminNeedsMFASetup {
		resp["mfa_setup_required"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateBucketJSON(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	bucket := r.PathValue("bucket")
	owner := info.Username
	if auth.CanSeeAllBuckets(info.Role) && r.URL.Query().Get("owner") != "" {
		owner = r.URL.Query().Get("owner")
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.svc.CreateBucket(r.Context(), bucket, owner); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, metadata.ErrBucketExists) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	vis := req.Visibility
	if vis == "" {
		vis = "private"
	}
	if vis == "private" || vis == "public-read" {
		_ = s.applyBucketVisibilityPolicy(info, bucket, vis)
	}
	s.stampBucketOwnership(bucket, info)
	s.logActivity(r, metadata.ActionBucketCreated, "bucket", bucket)
	s.emitEvent(metadata.EventBucketCreated, map[string]any{"bucket": bucket, "owner": owner})
	writeJSON(w, http.StatusCreated, map[string]any{"bucket": bucket, "visibility": vis})
}

func (s *Server) handleDeleteBucketJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, _, err := s.bucketStorageKey(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err := s.svc.DeleteBucket(r.Context(), sk); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionBucketDeleted, "bucket", bucket)
	s.emitEvent(metadata.EventBucketDeleted, map[string]any{"bucket": bucket})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUploadObjectJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	info, _ := authFrom(r)
	if !s.canWriteObjectKey(info, bucket, key) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, _, err := s.bucketStorageKey(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var prevSize int64
	if prev, err := s.meta.GetObject(sk, key); err == nil && !prev.IsDeleteMarker {
		prevSize = prev.Size
	}
	rec, err := s.svc.PutObject(r.Context(), sk, key, r.Body, r.ContentLength, contentType, nil)
	if err != nil {
		if err == metadata.ErrQuotaExceeded {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "quota exceeded"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	uploadDelta := rec.Size - prevSize
	if uploadDelta > 0 {
		_ = s.meta.AddUsageBytes(uploadDelta, 0)
	} else if uploadDelta < 0 {
		_ = s.meta.AddUsageBytes(0, -uploadDelta)
	}
	s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+key)
	s.emitEvent(metadata.EventObjectCreated, map[string]any{"bucket": bucket, "key": key, "size": rec.Size})
	writeJSON(w, http.StatusOK, map[string]any{"object": rec})
}

func (s *Server) handleDeleteObjectJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	info, _ := authFrom(r)
	if !s.canWriteObjectKey(info, bucket, key) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, _, err := s.bucketStorageKey(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	obj, err := s.meta.GetObject(sk, key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	schedule := r.URL.Query().Get("schedule")
	if schedule != "" {
		deleteAt, err := parseScheduledDelete(schedule)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		obj.ScheduledDeleteAt = &deleteAt
		if err := s.meta.PutObject(obj); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.logActivity(r, metadata.ActionObjectScheduledDelete, "object", bucket+"/"+key)
		writeJSON(w, http.StatusOK, map[string]any{
			"scheduled_delete_at": deleteAt.UTC().Format(time.RFC3339),
		})
		return
	}
	s.deleteObjectNow(w, r, bucket, sk, key, r.URL.Query().Get("versionId"), obj)
}

func (s *Server) deleteObjectNow(w http.ResponseWriter, r *http.Request, logicalBucket, storageKey, key, versionID string, obj metadata.ObjectRecord) {
	if err := s.checkObjectDeletable(storageKey, key, versionID); err != nil {
		if err == metadata.ErrLegalHold {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "object is under legal hold"})
			return
		}
		if err == metadata.ErrRetentionLocked {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "retention period has not expired"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if moved, tr, err := s.maybeSoftDelete(r, storageKey, key, versionID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	} else if moved {
		s.logActivity(r, metadata.ActionObjectDeleted, "object", logicalBucket+"/"+key)
		s.emitEvent(metadata.EventObjectDeleted, map[string]any{"bucket": logicalBucket, "key": key, "trash_id": tr.ID})
		writeJSON(w, http.StatusOK, map[string]any{"trashed": true, "trash_id": tr.ID})
		return
	}
	if err := s.svc.DeleteObject(r.Context(), storageKey, key, versionID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	if obj.Size > 0 {
		_ = s.meta.AddUsageBytes(0, obj.Size)
	}
	s.logActivity(r, metadata.ActionObjectDeleted, "object", logicalBucket+"/"+key)
	s.emitEvent(metadata.EventObjectDeleted, map[string]any{"bucket": logicalBucket, "key": key})
	w.WriteHeader(http.StatusNoContent)
}

func parseScheduledDelete(schedule string) (time.Time, error) {
	now := time.Now().UTC()
	switch schedule {
	case "1d":
		return now.Add(24 * time.Hour), nil
	case "1w":
		return now.Add(7 * 24 * time.Hour), nil
	case "1m":
		return now.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, errors.New("schedule must be 1d, 1w, or 1m")
	}
}

func (s *Server) handleListBucketsJSON(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	buckets, err := s.meta.ListBucketsFiltered(s.bucketListFilter(info))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	type bucketItem struct {
		metadata.BucketRecord
		Access bucketAccessInfo `json:"access"`
	}
	out := make([]bucketItem, 0, len(buckets))
	for _, b := range buckets {
		access := s.bucketAccessForUser(info, b)
		if filter != "" && filter != "all" && access.Ownership != filter {
			continue
		}
		out = append(out, bucketItem{BucketRecord: b, Access: access})
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": out})
}

func (s *Server) handleListObjectsJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	sk := rec.EffectiveStorageKey()
	prefix := r.URL.Query().Get("prefix")
	if eff, ok := s.allowedListPrefix(info, rec, prefix); !ok {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	} else {
		prefix = eff
	}
	_ = s.meta.TouchRecentItem(info.UserID, bucket, prefix)
	delimiter := r.URL.Query().Get("delimiter")
	startAfter := r.URL.Query().Get("start_after")
	maxKeys := parseMaxKeys(r, 100, 1000)
	if delimiter != "" {
		all, err := s.meta.ListObjects(sk, prefix, 0)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
			return
		}
		all = s.filterObjectsForPrefixAccess(info, rec, all)
		if prefix == "" {
			if roots := s.prefixGrantFolderRoots(info, rec); len(roots) > 0 {
				writeJSON(w, http.StatusOK, map[string]any{
					"folders": roots, "objects": []metadata.ObjectRecord{},
					"truncated": false, "next_marker": "",
				})
				return
			}
		}
		folders, files, truncated, nextMarker := paginateDelimitedListing(all, prefix, startAfter, maxKeys)
		writeJSON(w, http.StatusOK, map[string]any{
			"folders": folders, "objects": files,
			"truncated": truncated, "next_marker": nextMarker,
		})
		return
	}
	objs, truncated, nextMarker, err := s.meta.ListObjectsPage(sk, prefix, startAfter, maxKeys)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	objs = s.filterObjectsForPrefixAccess(info, rec, objs)
	writeJSON(w, http.StatusOK, map[string]any{
		"objects": objs, "truncated": truncated, "next_marker": nextMarker,
	})
}

func (s *Server) handlePresign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method   string `json:"method"`
		Bucket   string `json:"bucket"`
		Key      string `json:"key"`
		Expires  int    `json:"expires_seconds"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.Expires <= 0 {
		req.Expires = 3600
	}
	if req.Endpoint == "" {
		req.Endpoint = "http://" + r.Host
	}
	creds := auth.Credentials{AccessKey: s.cfg.AccessKey, SecretKey: s.cfg.SecretKey}
	url, err := s.svc.Signer.PresignURL(req.Method, req.Endpoint, req.Bucket, req.Key, creds, time.Duration(req.Expires)*time.Second)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	keys, err := s.meta.ListAccessKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type safe struct {
		AccessKey string    `json:"access_key"`
		Label     string    `json:"label"`
		Owner     string    `json:"owner,omitempty"`
		CreatedAt time.Time `json:"created_at"`
	}
	var out []safe
	for _, k := range keys {
		if !auth.IsAdmin(info.Role) {
			if k.Owner != "" && k.Owner != info.Username && k.OwnerID != info.UserID {
				continue
			}
			if k.Owner == "" && k.OwnerID == "" {
				continue
			}
		}
		out = append(out, safe{AccessKey: k.AccessKey, Label: k.Label, Owner: k.Owner, CreatedAt: k.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	accessKey := "AKIA" + randomHex(8)
	secretKey := randomHex(16)
	rec := metadata.AccessKeyRecord{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Label:     req.Label,
		OwnerID:   info.UserID,
		Owner:     info.Username,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutAccessKey(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionAccessKeyCreated, "access_key", accessKey)
	writeJSON(w, http.StatusCreated, map[string]any{
		"access_key": accessKey,
		"secret_key": secretKey,
		"label":      req.Label,
	})
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	accessKey := r.PathValue("accessKey")
	if accessKey == s.cfg.AccessKey {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot delete bootstrap key"})
		return
	}
	rec, err := s.meta.GetAccessKey(accessKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !auth.IsAdmin(info.Role) && rec.Owner != info.Username && rec.OwnerID != info.UserID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.meta.DeleteAccessKey(accessKey); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	s.logActivity(r, metadata.ActionAccessKeyDeleted, "access_key", accessKey)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": rec.Policy})
}

func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Policy string `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := s.meta.SetBucketPolicy(rec.EffectiveStorageKey(), req.Policy); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	s.logActivity(r, metadata.ActionPolicyChanged, "bucket", bucket)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetLifecycle(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rec.LifecycleRules})
}

func (s *Server) handlePutLifecycle(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Rules []metadata.LifecycleRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := s.meta.SetBucketLifecycle(rec.EffectiveStorageKey(), req.Rules); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	s.logActivity(r, metadata.ActionBucketUpdated, "bucket", bucket)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n*2]
}

// AdminToken issues a JWT for tests.
func (s *Server) AdminToken() (string, error) {
	return s.jwt.Issue(auth.TokenInfo{
		Username: s.cfg.AdminUser,
		UserID:   "admin-bootstrap",
		Role:     auth.RoleAdministrator,
	})
}

// PruneLifecycleOnce runs one lifecycle expiration sweep (for tests).
func (s *Server) PruneLifecycleOnce(ctx context.Context) {
	s.svc.PruneExpiredOnce(ctx)
}

// PruneScheduledDeletesOnce runs one scheduled-deletion sweep (for tests).
func (s *Server) PruneScheduledDeletesOnce(ctx context.Context) {
	deleted := s.svc.PruneScheduledDeletesOnce(ctx)
	for _, obj := range deleted {
		if obj.Size > 0 {
			_ = s.meta.AddUsageBytes(0, obj.Size)
		}
		s.logActivityAs("system", "", metadata.ActionObjectDeleted, "object", obj.Bucket+"/"+obj.Key)
	}
}

// SetObjectModified backdates object metadata (for tests).
func (s *Server) SetObjectModified(bucket, key string, mod time.Time) error {
	rec, err := s.meta.GetObject(bucket, key)
	if err != nil {
		return err
	}
	rec.LastModified = mod
	return s.meta.PutObject(rec)
}
