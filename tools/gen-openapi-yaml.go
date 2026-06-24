//go:build ignore

// Generates docs/api/openapi.yaml and internal/openapi/openapi.yaml from route inventory.
// Run: go run tools/gen-openapi-yaml.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	community := buildSpec(false)
	full := buildSpec(true)
	targets := []struct {
		path    string
		content string
	}{
		{filepath.Join(root, "docs", "api", "openapi.yaml"), community},
		{filepath.Join(root, "internal", "openapi", "openapi.yaml"), community},
		{filepath.Join(root, "docs", "api", "openapi-full.yaml"), full},
	}
	for _, t := range targets {
		if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(t.path, []byte(t.content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", t.path, err)
			os.Exit(1)
		}
		fmt.Println("wrote", t.path)
	}
}

func buildSpec(full bool) string {
	var b strings.Builder
	if full {
		b.WriteString(fullHeader)
	} else {
		b.WriteString(communityHeader)
	}
	comp := components
	if full {
		comp = strings.Replace(components, "\n  parameters:", "\n"+jwtSecurityScheme+"\n  parameters:", 1)
	}
	b.WriteString(comp)
	byPath := make(map[string][]opDef)
	var pathOrder []string
	for _, op := range operations {
		if !full && !isCommunityOp(op) {
			continue
		}
		if _, ok := byPath[op.path]; !ok {
			pathOrder = append(pathOrder, op.path)
		}
		byPath[op.path] = append(byPath[op.path], op)
	}
	for _, path := range pathOrder {
		writePath(&b, path, byPath[path], full)
	}
	b.WriteString("tags:\n")
	tagList := communityTags
	if full {
		tagList = fullTags
	}
	for _, t := range tagList {
		fmt.Fprintf(&b, "  - name: %s\n    description: %s\n", t.name, t.desc)
	}
	return b.String()
}

type tag struct{ name, desc string }

var communityTags = []tag{
	{"Health", "Health probe"},
	{"Profile", "Current user profile"},
	{"Buckets", "Bucket and object operations"},
	{"Keys", "S3 access keys and presign"},
	{"Shares", "Share links and public download"},
	{"Tokens", "Integration API tokens (ds_*)"},
	{"Usage", "Storage usage (scoped to caller)"},
	{"Search", "Search, favorites, tags, trash"},
	{"Lifecycle", "Bucket lifecycle and object lock"},
}

var fullTags = []tag{
	{"Health", "Health and readiness probes"},
	{"Auth", "Login, MFA, OIDC browser flows, logout"},
	{"Profile", "Current user profile and password"},
	{"Buckets", "Buckets, objects, multipart, folders"},
	{"Keys", "S3 access keys and presigned URLs"},
	{"Shares", "Share links and anonymous public download"},
	{"Tokens", "Console API tokens (ds_*)"},
	{"Usage", "Storage usage and quotas"},
	{"Search", "Search, favorites, tags, trash, metadata"},
	{"Lifecycle", "Lifecycle rules, legal hold, retention"},
	{"Admin", "Users, activity, bucket policies, settings"},
	{"Webhooks", "Event webhooks and delivery log"},
	{"Tenants", "Multi-tenant isolation, groups, members, grants"},
	{"Gateway", "Replication to external S3, connections, sync jobs"},
	{"Federation", "Federation clusters and cluster status"},
	{"Enterprise", "LDAP admin, MFA enrollment, STS"},
}

func isCommunityOp(op opDef) bool {
	excluded := []string{
		"/admin/",
		"/settings/",
		"/users",
		"/webhooks",
		"/activity",
		"/tenants",
		"/gateway/",
		"/federation/",
		"/cluster/",
		"/auth/",
		"/mfa/",
		"/sts/",
	}
	for _, p := range excluded {
		if strings.HasPrefix(op.path, p) {
			return false
		}
	}
	if op.path == "/buckets/{bucket}/policy" {
		return false
	}
	return true
}

type opDef struct {
	method, path, id, summary, tag, security string
	priority                                   string
	bodySchema                                 string
	responses                                  string
	params                                     string
}

const communityHeader = `openapi: 3.1.0
info:
  title: DataSafeS3 Community Integration API
  version: 1.0.0
  summary: Machine-facing JSON REST API for integrators (subset of /api/v1)
  description: |
    **DataSafeS3 Community edition** — JSON REST Integration API for machine clients and integrators.

    ## Authentication
    Use an **API token** (prefix ds_*) in header Authorization Bearer ds_...
    Create tokens in the web console: **Access → API tokens → Create** (human login once).

    ## Out of scope
    - Admin routes (users, system settings, webhooks, tenants, gateway) — see openapi-full.yaml
    - AWS **S3 XML API** (SigV4) on port **9000** — use AWS SDKs

    ## Errors
    JSON object with error field; delete may return scheduled_delete_at, trashed, trash_id.

    Docs: [Swagger UI guide](../../docs/en/api/swagger.md) · [Full API spec](../../api/openapi-full.yaml)
  contact:
    name: DataSafeS3 / Ilya Trachuk
    url: https://github.com/DirektorBani/DataSafeS3
  license:
    name: Apache 2.0
    identifier: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0

servers:
  - url: http://localhost:8080/api/v1
    description: Console origin via Caddy (Swagger UI at /api/v1/docs)
  - url: http://localhost:9000/api/v1
    description: storage-server direct

security:
  - BearerAPIToken: []

`

const fullHeader = `openapi: 3.1.0
info:
  title: DataSafeS3 REST Admin API
  version: 1.0.0
  summary: Complete JSON REST API served at /api/v1 (excluding S3 XML)
  description: |
    **Full hand-written OpenAPI 3.1** for all Admin JSON routes registered in internal/api/server.go.

    ## Authentication
    | Scheme | Use |
    |--------|-----|
    | **BearerAPIToken** | Integration tokens ds_* (recommended for automation) |
    | **BearerJWT** | JWT from POST /admin/login or OIDC callback (console sessions) |

    Public endpoints: GET /health, POST /admin/login, OIDC bootstrap, GET /public/share/*.

    ## Not included
    - GET /metrics (Prometheus text)
    - S3 XML API on paths outside /api/v1 (SigV4, port 9000)

    ## Errors
    Standard envelope: JSON object with error field (HTTP 4xx/5xx).

    Interactive docs for **Community subset**: GET /api/v1/docs (Swagger UI).

    Canonical file: [openapi-full.yaml](../../api/openapi-full.yaml)
  contact:
    name: DataSafeS3 / Ilya Trachuk
    url: https://github.com/DirektorBani/DataSafeS3
  license:
    name: Apache 2.0
    identifier: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0

servers:
  - url: http://localhost:8080/api/v1
    description: Via Caddy (console origin)
  - url: http://localhost:9000/api/v1
    description: storage-server direct

security:
  - BearerAPIToken: []
  - BearerJWT: []

`

const components = `components:
  securitySchemes:
    BearerAPIToken:
      type: http
      scheme: bearer
      bearerFormat: ds_
      description: |
        Integration API token with prefix ds_. Create in console: Access → API tokens.
        Inherits creator's role and bucket access.

  parameters:
    BucketName:
      name: bucket
      in: path
      required: true
      schema: { type: string }
    ObjectKey:
      name: key
      in: path
      required: true
      schema: { type: string }
      description: Object key (may contain slashes)
    Offset:
      name: offset
      in: query
      schema: { type: integer, minimum: 0, default: 0 }
    Limit:
      name: limit
      in: query
      schema: { type: integer, minimum: 1, maximum: 1000, default: 50 }
    Prefix:
      name: prefix
      in: query
      schema: { type: string }
    Delimiter:
      name: delimiter
      in: query
      schema: { type: string, example: "/" }
    StartAfter:
      name: start_after
      in: query
      schema: { type: string }
    MaxKeys:
      name: max_keys
      in: query
      schema: { type: integer, minimum: 1, maximum: 1000, default: 100 }

  schemas:
    ErrorResponse:
      type: object
      required: [error]
      properties:
        error: { type: string }
        mfa_required: { type: boolean }
        mfa_token: { type: string }
        mfa_setup_required: { type: boolean }
        scheduled_delete_at: { type: string, format: date-time }
        trashed: { type: boolean }
        trash_id: { type: string }

    HealthStatus:
      type: object
      properties:
        status: { type: string, example: ok }

    LoginRequest:
      type: object
      required: [username, password]
      properties:
        username: { type: string }
        password: { type: string, format: password }
        mfa_code: { type: string }

    LoginResponse:
      type: object
      properties:
        token: { type: string }
        expires_in: { type: integer, example: 86400 }
        username: { type: string }
        role: { type: string, enum: [administrator, operator, user] }
        user_id: { type: string }
        mfa_enabled: { type: boolean }
        mfa_required: { type: boolean }
        mfa_token: { type: string }

    MFALoginRequest:
      type: object
      required: [mfa_token, mfa_code]
      properties:
        mfa_token: { type: string }
        mfa_code: { type: string }

    MeResponse:
      type: object
      properties:
        username: { type: string }
        email: { type: string }
        role: { type: string }
        status: { type: string }
        user_id: { type: string }
        tenant_id: { type: string }
        mfa_enabled: { type: boolean }
        auth_source: { type: string }
        is_tenant_admin: { type: boolean }
        tenant_memberships:
          type: array
          items:
            type: object
            properties:
              tenant_id: { type: string }
              tenant_name: { type: string }
              role: { type: string }

    Bucket:
      type: object
      properties:
        name: { type: string }
        storage_key: { type: string }
        owner: { type: string }
        owner_id: { type: string }
        tenant_id: { type: string }
        created_at: { type: string, format: date-time }
        visibility: { type: string, enum: [private, public-read] }
        access:
          type: object
          properties:
            ownership: { type: string, enum: [owned, shared, tenant] }
            can_read: { type: boolean }
            can_write: { type: boolean }
            shared_by: { type: string, nullable: true }
        versioning_enabled: { type: boolean }
        object_lock_enabled: { type: boolean }
        description: { type: string }
        tags: { type: object, additionalProperties: { type: string } }

    Object:
      type: object
      properties:
        bucket: { type: string }
        key: { type: string }
        size: { type: integer, format: int64 }
        etag: { type: string }
        content_type: { type: string }
        version_id: { type: string }
        last_modified: { type: string, format: date-time }
        legal_hold: { type: boolean }
        retention_until: { type: string, format: date-time }
        metadata: { type: object, additionalProperties: { type: string } }
        tags: { type: object, additionalProperties: { type: string } }

    ObjectListResponse:
      type: object
      properties:
        objects:
          type: array
          items: { $ref: '#/components/schemas/Object' }
        folders:
          type: array
          items: { type: string }
        truncated: { type: boolean }
        next_marker: { type: string }

    AccessKey:
      type: object
      properties:
        access_key: { type: string }
        label: { type: string }
        owner: { type: string }
        created_at: { type: string, format: date-time }

    ShareLink:
      type: object
      properties:
        id: { type: string }
        bucket: { type: string }
        key: { type: string }
        token: { type: string }
        max_downloads: { type: integer }
        download_count: { type: integer }
        created_by: { type: string }
        created_at: { type: string, format: date-time }
        expires_at: { type: string, format: date-time }

    APIToken:
      type: object
      properties:
        id: { type: string }
        name: { type: string }
        username: { type: string }
        scopes:
          type: array
          items: { type: string }
        expires_at: { type: string, format: date-time }
        created_at: { type: string, format: date-time }

    OkResponse:
      type: object
      properties:
        ok: { type: boolean }

    User:
      type: object
      properties:
        id: { type: string }
        username: { type: string }
        email: { type: string }
        role: { type: string, enum: [administrator, operator, user] }
        status: { type: string }
        tenant_id: { type: string }
        mfa_enabled: { type: boolean }

    CreateShareRequest:
      type: object
      required: [key]
      properties:
        key: { type: string, description: Object key to share }
        expires_in_sec: { type: integer, minimum: 0, description: "TTL; 0 = no expiry" }
        max_downloads: { type: integer, minimum: 0, default: 0 }

    CreateTokenRequest:
      type: object
      required: [name]
      properties:
        name: { type: string }
        expires_days: { type: integer, minimum: 1, default: 90 }
        scopes: { type: array, items: { type: string } }

    CreateTokenResponse:
      type: object
      properties:
        id: { type: string }
        token: { type: string, description: Plain token shown once }
        name: { type: string }
        expires_at: { type: string, format: date-time }

    PresignRequest:
      type: object
      required: [bucket, key]
      properties:
        method: { type: string, enum: [GET, PUT, HEAD, DELETE], default: GET }
        bucket: { type: string }
        key: { type: string }
        expires_seconds: { type: integer, minimum: 1, default: 3600 }
        endpoint: { type: string, format: uri, description: S3 base URL, default request Host }

    PresignResponse:
      type: object
      properties:
        url: { type: string, format: uri }

    CreateKeyRequest:
      type: object
      properties:
        label: { type: string }

    CreateKeyResponse:
      type: object
      properties:
        access_key: { type: string }
        secret_key: { type: string, description: Shown once }
        label: { type: string }

    ChangePasswordRequest:
      type: object
      required: [current_password, new_password]
      properties:
        current_password: { type: string, format: password }
        new_password: { type: string, format: password, minLength: 6 }

    HooksTestRequest:
      type: object
      required: [url]
      properties:
        url: { type: string, format: uri }
        event: { type: string, example: extension.test }
        secret: { type: string }

    BulkDeleteRequest:
      type: object
      required: [keys]
      properties:
        keys: { type: array, items: { type: string }, minItems: 1 }

    TagsMap:
      type: object
      additionalProperties: { type: string }

    LifecycleRules:
      type: object
      properties:
        rules:
          type: array
          items:
            type: object
            properties:
              id: { type: string }
              prefix: { type: string }
              enabled: { type: boolean }
              expiration_days: { type: integer }

  examples:
    HealthOk:
      value: { status: ok }
    ErrorUnauthorized:
      value: { error: unauthorized }
    CreateShareBody:
      value: { key: reports/2024/q1.pdf, expires_in_sec: 86400, max_downloads: 10 }

  responses:
    BadRequest:
      description: Invalid request
      content:
        application/json:
          schema: { $ref: '#/components/schemas/ErrorResponse' }
    Unauthorized:
      description: Missing or invalid credentials
      content:
        application/json:
          schema: { $ref: '#/components/schemas/ErrorResponse' }
    Forbidden:
      description: Insufficient permissions
      content:
        application/json:
          schema: { $ref: '#/components/schemas/ErrorResponse' }
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema: { $ref: '#/components/schemas/ErrorResponse' }
    InternalError:
      description: Server error
      content:
        application/json:
          schema: { $ref: '#/components/schemas/ErrorResponse' }

paths:
`

const jwtSecurityScheme = `    BearerJWT:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: |
        HS256 JWT from POST /admin/login, MFA completion, or OIDC callback.
`

var operations = []opDef{
	{"get", "/health", "healthCheck", "Health check", "P0 Auth", "", "P0", "", `        '200':
          description: OK
          content:
            application/json:
              schema: { $ref: '#/components/schemas/HealthStatus' }`, ""},

	{"post", "/admin/login", "adminLogin", "Sign in (local/LDAP)", "P0 Auth", "", "P0", "LoginRequest", `        '200':
          description: JWT issued or MFA step required
          content:
            application/json:
              schema: { $ref: '#/components/schemas/LoginResponse' }
        '401': { $ref: '#/components/responses/Unauthorized' }`, ""},
	{"post", "/mfa/login", "mfaLogin", "Complete MFA login (alias)", "P0 Auth", "", "P0", "MFALoginRequest", `        '200':
          description: JWT issued
          content:
            application/json:
              schema: { $ref: '#/components/schemas/LoginResponse' }`, ""},
	{"post", "/auth/login/mfa", "authLoginMfa", "Complete MFA login", "P0 Auth", "", "P0", "MFALoginRequest", `        '200':
          description: JWT issued
          content:
            application/json:
              schema: { $ref: '#/components/schemas/LoginResponse' }`, ""},

	{"get", "/auth/oidc/config", "oidcConfig", "OIDC public config", "P2 Enterprise", "", "P2", "", `        '200': { description: OIDC client config }`, ""},
	{"get", "/auth/oidc/login", "oidcLogin", "Start OIDC redirect", "P2 Enterprise", "", "P2", "", `        '302': { description: Redirect to IdP }`, ""},
	{"post", "/auth/oidc/password-login", "oidcPasswordLogin", "OIDC resource-owner password (test)", "P2 Enterprise", "", "P2", "", `        '200': { description: JWT }`, ""},

	{"get", "/me", "getMe", "Current user profile", "P0 Auth", "jwt", "P0", "", `        '200':
          description: Profile
          content:
            application/json:
              schema: { $ref: '#/components/schemas/MeResponse' }`, ""},
	{"post", "/me/password", "changePassword", "Change password", "P0 Auth", "jwt", "P0", "ChangePasswordRequest", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/OkResponse' }`, ""},
	{"post", "/me/mfa/webauthn/register/begin", "webauthnRegisterBegin", "Begin WebAuthn passkey enrollment", "P1 Admin", "jwt", "P1", "", `        '200': { description: PublicKeyCredentialCreationOptions JSON }`, ""},
	{"post", "/me/mfa/webauthn/register/finish", "webauthnRegisterFinish", "Finish WebAuthn passkey enrollment", "P1 Admin", "jwt", "P1", "", `        '200': { description: Passkey enrolled }`, ""},
	{"post", "/admin/logout", "logout", "Sign out", "P0 Auth", "jwt", "P0", "", `        '204': { description: Logged out }`, ""},

	{"get", "/buckets", "listBuckets", "List buckets", "P0 Buckets", "any", "P0", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  buckets:
                    type: array
                    items: { $ref: '#/components/schemas/Bucket' }`, "    - name: filter\n      in: query\n      schema: { type: string, enum: [owned, shared, tenant, all] }\n      description: Filter by access.ownership (default all)\n"},
	{"post", "/buckets/{bucket}", "createBucket", "Create bucket", "P0 Buckets", "any", "P0", "", `        '201':
          description: Created with visibility
          content:
            application/json:
              schema:
                type: object
                properties:
                  bucket: { type: string }
                  visibility: { type: string }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"delete", "/buckets/{bucket}", "deleteBucket", "Delete empty bucket", "P0 Buckets", "any", "P0", "", `        '204': { description: Deleted }`, "    - $ref: '#/components/parameters/BucketName'\n"},

	{"get", "/buckets/{bucket}/access", "listBucketAccessByBucket", "List bucket access grants (owner or tenant admin)", "P0 Buckets", "any", "P0", "", `        '200': { description: Grants }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/access", "putBucketAccessByBucket", "Replace bucket access grants", "P0 Buckets", "any", "P0", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"delete", "/buckets/{bucket}/access/{user_id}", "deleteBucketAccessByBucket", "Revoke bucket access for user", "P0 Buckets", "any", "P0", "", `        '204': { description: Revoked }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: user_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/shareable-users", "listShareableUsers", "Users that can receive bucket grants", "P0 Buckets", "any", "P0", "", `        '200': { description: Users }`, "    - name: bucket\n      in: query\n      required: true\n      schema: { type: string }\n    - name: q\n      in: query\n      schema: { type: string }\n      description: Username search (max 50 results)\n"},
	{"get", "/notifications", "listNotifications", "In-app notifications for current user", "P0 Buckets", "any", "P0", "", `        '200': { description: Notifications }`, ""},
	{"post", "/notifications/{id}/read", "markNotificationRead", "Mark notification as read", "P0 Buckets", "any", "P0", "", `        '204': { description: Read }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/notifications/read-all", "markAllNotificationsRead", "Mark all notifications read", "P0 Buckets", "any", "P0", "", `        '204': { description: Read }`, ""},
	{"get", "/recent", "listRecent", "Recently accessed buckets and folders", "P0 Buckets", "any", "P0", "", `        '200': { description: Recent items }`, ""},

	{"get", "/buckets/{bucket}/objects", "listObjects", "List objects", "P0 Buckets", "any", "P0", "", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ObjectListResponse' }`, "    - $ref: '#/components/parameters/BucketName'\n    - $ref: '#/components/parameters/Prefix'\n    - $ref: '#/components/parameters/Delimiter'\n    - $ref: '#/components/parameters/StartAfter'\n    - $ref: '#/components/parameters/MaxKeys'\n"},
	{"get", "/buckets/{bucket}/versions", "listObjectVersions", "List object versions", "P0 Buckets", "any", "P0", "", `        '200': { description: Version list }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"get", "/buckets/{bucket}/settings", "getBucketSettings", "Get bucket settings", "P0 Buckets", "any", "P0", "", `        '200': { description: Bucket settings }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"get", "/buckets/{bucket}/object-meta", "getObjectMeta", "Get object metadata", "P1 Search", "any", "P1", "", `        '200': { description: Object metadata }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: key\n      in: query\n      required: true\n      schema: { type: string }\n"},
	{"put", "/buckets/{bucket}/object-meta", "putObjectMeta", "Update object metadata", "P1 Search", "any", "P1", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"post", "/buckets/{bucket}/folders", "createFolder", "Create folder placeholder", "P0 Buckets", "any", "P0", "", `        '201': { description: Folder created }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"delete", "/buckets/{bucket}/folders", "deleteFolder", "Delete folder", "P0 Buckets", "any", "P0", "", `        '200': { description: Deleted or conflict with object_count }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"post", "/buckets/{bucket}/bulk-delete", "bulkDeleteObjects", "Bulk delete objects", "P0 Buckets", "any", "P0", "BulkDeleteRequest", `        '200': { description: Bulk result }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"post", "/buckets/{bucket}/object-actions", "objectActions", "Batch object actions", "P0 Buckets", "any", "P0", "", `        '200': { description: Action results }`, "    - $ref: '#/components/parameters/BucketName'\n"},

	{"get", "/buckets/{bucket}/objects/{key}", "downloadObject", "Download object", "P0 Buckets", "any", "P0", "", `        '200':
          description: Object bytes
          content:
            application/octet-stream:
              schema: { type: string, format: binary }`, "    - $ref: '#/components/parameters/BucketName'\n    - $ref: '#/components/parameters/ObjectKey'\n"},
	{"put", "/buckets/{bucket}/objects/{key}", "uploadObject", "Upload object", "P0 Buckets", "any", "P0", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  object: { $ref: '#/components/schemas/Object' }`, "    - $ref: '#/components/parameters/BucketName'\n    - $ref: '#/components/parameters/ObjectKey'\n"},
	{"delete", "/buckets/{bucket}/objects/{key}", "deleteObject", "Delete or schedule delete object", "P0 Buckets", "any", "P0", "", `        '204': { description: Deleted }
        '200': { description: Scheduled delete or soft-delete to trash }`, "    - $ref: '#/components/parameters/BucketName'\n    - $ref: '#/components/parameters/ObjectKey'\n    - name: schedule\n      in: query\n      schema: { type: string, enum: [1d, 1w, 1m] }\n    - name: versionId\n      in: query\n      schema: { type: string }\n"},

	{"post", "/presign", "presignURL", "Generate presigned S3 URL", "P0 Keys", "any", "P0", "PresignRequest", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/PresignResponse' }`, ""},
	{"get", "/keys", "listKeys", "List S3 access keys", "P0 Keys", "any", "P0", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  keys:
                    type: array
                    items: { $ref: '#/components/schemas/AccessKey' }`, ""},
	{"post", "/keys", "createKey", "Create S3 access key", "P0 Keys", "any", "P0", "CreateKeyRequest", `        '201':
          description: access_key and secret_key returned once
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CreateKeyResponse' }`, ""},
	{"delete", "/keys/{accessKey}", "deleteKey", "Delete access key", "P0 Keys", "any", "P0", "", `        '204': { description: Deleted }`, "    - name: accessKey\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/buckets/{bucket}/policy", "getBucketPolicy", "Get bucket policy JSON", "P1 Admin", "admin", "P1", "", `        '200': { description: policy document }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/policy", "putBucketPolicy", "Set bucket policy", "P1 Admin", "admin", "P1", "", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/OkResponse' }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"get", "/buckets/{bucket}/lifecycle", "getLifecycle", "Get lifecycle rules", "P1 Admin", "any", "P1", "", `        '200': { description: rules array }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/lifecycle", "putLifecycle", "Set lifecycle rules", "P1 Admin", "any", "P1", "LifecycleRules", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/OkResponse' }`, "    - $ref: '#/components/parameters/BucketName'\n"},

	{"get", "/activity", "listActivity", "Audit activity log", "P1 Admin", "admin", "P1", "", `        '200': { description: Paginated activity }`, "    - $ref: '#/components/parameters/Offset'\n    - $ref: '#/components/parameters/Limit'\n"},
	{"get", "/usage", "getUsage", "Storage usage statistics", "P0 Tokens", "any", "P0", "", `        '200': { description: Usage aggregates }`, ""},

	{"get", "/users", "listUsers", "List users", "P1 Admin", "any", "P1", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  users:
                    type: array
                    items: { $ref: '#/components/schemas/User' }
                  total: { type: integer }
                  offset: { type: integer }
                  limit: { type: integer }`, "    - $ref: '#/components/parameters/Offset'\n    - $ref: '#/components/parameters/Limit'\n"},
	{"post", "/users", "createUser", "Create user", "P1 Admin", "admin", "P1", "", `        '201': { description: User created }`, ""},
	{"put", "/users/{id}", "updateUser", "Update user", "P1 Admin", "admin", "P1", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/users/{id}", "deleteUser", "Delete user", "P1 Admin", "admin", "P1", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/users/{id}/reset-password", "resetPassword", "Reset user password", "P1 Admin", "admin", "P1", "", `        '200': { description: New password }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/settings/buckets", "listBucketSettings", "List all bucket settings", "P1 Admin", "admin", "P1", "", `        '200': { description: Bucket settings list }`, ""},
	{"put", "/settings/buckets/{name}", "updateBucketSettings", "Update bucket settings by name", "P1 Admin", "admin", "P1", "", `        '200': { description: Updated }`, "    - name: name\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/settings/system", "getSystemConfig", "Get system configuration", "P1 Admin", "admin", "P1", "", `        '200': { description: SystemConfig object }`, ""},
	{"put", "/settings/system", "putSystemConfig", "Update system configuration", "P1 Admin", "admin", "P1", "", `        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/OkResponse' }`, ""},

	{"get", "/trash", "listTrash", "List trashed objects", "P1 Admin", "any", "P1", "", `        '200': { description: Trash items }`, "    - name: bucket\n      in: query\n      schema: { type: string }\n"},
	{"post", "/trash/{id}/restore", "restoreTrash", "Restore from trash", "P1 Admin", "any", "P1", "", `        '200': { description: Object restored }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/trash/{id}", "purgeTrash", "Permanently purge trash item", "P1 Admin", "any", "P1", "", `        '204': { description: Purged }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/tokens", "listTokens", "List API tokens", "P0 Tokens", "any", "P0", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  tokens:
                    type: array
                    items: { $ref: '#/components/schemas/APIToken' }`, ""},
	{"post", "/tokens", "createToken", "Create API token", "P0 Tokens", "any", "P0", "CreateTokenRequest", `        '201':
          description: token value returned once
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CreateTokenResponse' }`, ""},
	{"delete", "/tokens/{id}", "deleteToken", "Revoke API token", "P0 Tokens", "any", "P0", "", `        '204': { description: Revoked }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/webhooks", "listWebhooks", "List webhooks", "P1 Admin", "admin", "P1", "", `        '200': { description: Webhooks }`, ""},
	{"post", "/webhooks", "createWebhook", "Create webhook", "P1 Admin", "admin", "P1", "", `        '201': { description: Created }`, ""},
	{"put", "/webhooks/{id}", "updateWebhook", "Update webhook", "P1 Admin", "admin", "P1", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/webhooks/{id}", "deleteWebhook", "Delete webhook", "P1 Admin", "admin", "P1", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/webhooks/templates", "webhookTemplates", "Webhook payload templates", "P1 Admin", "admin", "P1", "", `        '200': { description: Templates }`, ""},
	{"post", "/hooks/test", "hooksTest", "Send extension diagnostic webhook", "P1 Admin", "admin", "P1", "HooksTestRequest", `        '200': { description: Delivery result }`, ""},
	{"get", "/webhooks/{id}/deliveries", "listWebhookDeliveries", "Webhook delivery log", "P1 Admin", "admin", "P1", "", `        '200': { description: Deliveries }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: limit\n      in: query\n      schema: { type: integer, default: 50 }\n"},
	{"post", "/webhooks/{id}/deliveries/{deliveryId}/retry", "retryWebhookDelivery", "Retry failed delivery", "P1 Admin", "admin", "P1", "", `        '200': { description: Retry queued }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: deliveryId\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/search", "search", "Global search", "P1 Search", "any", "P1", "", `        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  total: { type: integer }
                  offset: { type: integer }
                  limit: { type: integer }
                  results: { type: array, items: { type: object } }`, "    - name: q\n      in: query\n      required: true\n      schema: { type: string }\n    - $ref: '#/components/parameters/Offset'\n    - $ref: '#/components/parameters/Limit'\n"},
	{"get", "/favorites", "listFavorites", "List favorites", "P1 Search", "any", "P1", "", `        '200': { description: Favorites }`, ""},
	{"post", "/favorites", "createFavorite", "Add favorite", "P1 Search", "any", "P1", "", `        '201': { description: Created }`, ""},
	{"delete", "/favorites/{id}", "deleteFavorite", "Remove favorite", "P1 Search", "any", "P1", "", `        '204': { description: Removed }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/buckets/{bucket}/tags", "getBucketTags", "Get bucket tags", "P1 Search", "any", "P1", "", `        '200': { description: Tags map }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/tags", "putBucketTags", "Set bucket tags", "P1 Search", "any", "P1", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"get", "/buckets/{bucket}/object-tags", "getObjectTags", "Get object tags", "P1 Search", "any", "P1", "", `        '200': { description: Tags }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/object-tags", "putObjectTags", "Set object tags", "P1 Search", "any", "P1", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},

	{"post", "/buckets/{bucket}/multipart", "initiateMultipart", "Initiate multipart upload", "P0 Buckets", "any", "P0", "", `        '200': { description: upload_id }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/buckets/{bucket}/multipart/{uploadId}/parts/{partNumber}", "uploadMultipartPart", "Upload multipart part", "P0 Buckets", "any", "P0", "", `        '200': { description: Part etag }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: uploadId\n      in: path\n      required: true\n      schema: { type: string }\n    - name: partNumber\n      in: path\n      required: true\n      schema: { type: integer }\n"},
	{"post", "/buckets/{bucket}/multipart/{uploadId}/complete", "completeMultipart", "Complete multipart upload", "P0 Buckets", "any", "P0", "", `        '200': { description: Object created }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: uploadId\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/buckets/{bucket}/multipart/{uploadId}", "abortMultipart", "Abort multipart upload", "P0 Buckets", "any", "P0", "", `        '204': { description: Aborted }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: uploadId\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"post", "/settings/ldap/test", "ldapTest", "Test LDAP connection", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Bind OK }`, ""},
	{"post", "/settings/ldap/sync", "ldapSync", "Sync LDAP users", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Sync result }`, ""},
	{"post", "/mfa/enroll", "mfaEnroll", "Start MFA enrollment", "P2 Enterprise", "jwt", "P2", "", `        '200': { description: QR secret }`, ""},
	{"post", "/mfa/verify-enroll", "mfaVerifyEnroll", "Verify MFA enrollment", "P2 Enterprise", "jwt", "P2", "", `        '200': { description: MFA enabled }`, ""},
	{"post", "/mfa/verify", "mfaVerify", "Verify MFA code", "P2 Enterprise", "jwt", "P2", "", `        '200': { description: Verified }`, ""},
	{"post", "/mfa/disable", "mfaDisable", "Disable MFA", "P2 Enterprise", "jwt", "P2", "", `        '200': { description: Disabled }`, ""},
	{"put", "/buckets/{bucket}/legal-hold", "setLegalHold", "Set object legal hold", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},

	{"get", "/buckets/{bucket}/shares", "listShares", "List share links", "P0 Shares", "any", "P0", "", `        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  shares:
                    type: array
                    items: { $ref: '#/components/schemas/ShareLink' }`, "    - $ref: '#/components/parameters/BucketName'\n    - name: key\n      in: query\n      schema: { type: string }\n"},
	{"post", "/buckets/{bucket}/shares", "createShare", "Create share link", "P0 Shares", "any", "P0", "CreateShareRequest", `        '201':
          description: Share created (expires_in_sec in request body)
          content:
            application/json:
              schema:
                type: object
                properties:
                  share: { $ref: '#/components/schemas/ShareLink' }
                  url: { type: string, format: uri }
        '404': { $ref: '#/components/responses/NotFound' }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"delete", "/shares/{id}", "revokeShare", "Revoke share link", "P0 Shares", "any", "P0", "", `        '204': { description: Revoked }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/public/share/{token}", "publicShareInfo", "Public share metadata", "P0 Shares", "", "P0", "", `        '200': { description: Share info (no auth) }`, "    - name: token\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/public/share/{token}/download", "publicShareDownload", "Download via share token", "P0 Shares", "", "P0", "", `        '200':
          description: File bytes
          content:
            application/octet-stream:
              schema: { type: string, format: binary }`, "    - name: token\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/tenants", "listTenants", "List tenants", "P2 Enterprise", "any", "P2", "", `        '200': { description: Tenants }`, ""},
	{"post", "/tenants", "createTenant", "Create tenant", "P2 Enterprise", "admin", "P2", "", `        '201': { description: Created }`, ""},
	{"delete", "/tenants/{id}", "deleteTenant", "Delete tenant", "P2 Enterprise", "admin", "P2", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/tenants/{id}/buckets", "listTenantBuckets", "List tenant buckets", "P2 Enterprise", "any", "P2", "", `        '200': { description: Buckets }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/tenants/{id}/groups", "listTenantGroups", "List tenant groups", "P2 Enterprise", "any", "P2", "", `        '200': { description: Groups }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/tenants/{id}/groups", "createTenantGroup", "Create tenant group", "P2 Enterprise", "any", "P2", "", `        '201': { description: Created }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/tenants/{id}/groups/{group_id}", "getTenantGroup", "Get tenant group", "P2 Enterprise", "any", "P2", "", `        '200': { description: Group }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: group_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"put", "/tenants/{id}/groups/{group_id}", "updateTenantGroup", "Update tenant group", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: group_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/tenants/{id}/groups/{group_id}", "deleteTenantGroup", "Delete tenant group", "P2 Enterprise", "any", "P2", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: group_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"put", "/tenants/{id}/groups/{group_id}/buckets", "putTenantGroupBuckets", "Assign buckets to group", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: group_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"put", "/tenants/{id}/members/{user_id}/groups", "putMemberGroups", "Assign member groups", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: user_id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/tenants/{id}/members", "listTenantMembers", "List tenant members", "P2 Enterprise", "any", "P2", "", `        '200': { description: Members }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/tenants/{id}/users", "createTenantUser", "Create user in tenant", "P2 Enterprise", "any", "P2", "", `        '201': { description: Created }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/tenants/{id}/members", "addTenantMember", "Add tenant member", "P2 Enterprise", "any", "P2", "", `        '201': { description: Added }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"put", "/tenants/{id}/members/{userId}", "updateTenantMember", "Update tenant member", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: userId\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"delete", "/tenants/{id}/members/{userId}", "removeTenantMember", "Remove tenant member", "P2 Enterprise", "any", "P2", "", `        '204': { description: Removed }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n    - name: userId\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/tenants/{tenant}/buckets/{bucket}/access", "listBucketAccess", "List bucket access grants", "P2 Enterprise", "any", "P2", "", `        '200': { description: Grants }`, "    - name: tenant\n      in: path\n      required: true\n      schema: { type: string }\n    - $ref: '#/components/parameters/BucketName'\n"},
	{"put", "/tenants/{tenant}/buckets/{bucket}/access", "putBucketAccess", "Set bucket access grants", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - name: tenant\n      in: path\n      required: true\n      schema: { type: string }\n    - $ref: '#/components/parameters/BucketName'\n"},
	{"delete", "/tenants/{tenant}/buckets/{bucket}/access/{user_id}", "deleteBucketAccess", "Revoke bucket access", "P2 Enterprise", "any", "P2", "", `        '204': { description: Revoked }`, "    - name: tenant\n      in: path\n      required: true\n      schema: { type: string }\n    - $ref: '#/components/parameters/BucketName'\n    - name: user_id\n      in: path\n      required: true\n      schema: { type: string }\n"},

	{"get", "/gateway/connections", "listGatewayConnections", "List gateway connections", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Connections }`, ""},
	{"post", "/gateway/connections", "createGatewayConnection", "Create gateway connection", "P2 Enterprise", "admin", "P2", "", `        '201': { description: Created }`, ""},
	{"delete", "/gateway/connections/{id}", "deleteGatewayConnection", "Delete connection", "P2 Enterprise", "admin", "P2", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/gateway/connections/{id}/test", "testGatewayConnection", "Test gateway connection", "P2 Enterprise", "admin", "P2", "", `        '200': { description: OK or error }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/gateway/replication", "listReplicationRules", "List replication rules", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Rules }`, ""},
	{"post", "/gateway/replication", "createReplicationRule", "Create replication rule", "P2 Enterprise", "admin", "P2", "", `        '201': { description: Created }`, ""},
	{"delete", "/gateway/replication/{id}", "deleteReplicationRule", "Delete replication rule", "P2 Enterprise", "admin", "P2", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"post", "/gateway/replication/{id}/sync", "triggerReplicationSync", "Trigger full sync job", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Job started }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/gateway/sync-jobs", "listSyncJobs", "List sync jobs", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Jobs }`, ""},
	{"get", "/gateway/health", "gatewayHealth", "Gateway health metrics", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Health }`, ""},
	{"get", "/gateway/replication/queue", "replicationQueue", "Replication task queue", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Queue }`, ""},
	{"post", "/gateway/replication/retry-failed", "retryFailedReplication", "Retry failed replication tasks", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Retried }`, ""},
	{"post", "/gateway/replication/clear-errors", "clearReplicationErrors", "Clear replication errors", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Cleared }`, ""},

	{"get", "/federation/clusters", "listFederationClusters", "List federation clusters", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Clusters }`, ""},
	{"post", "/federation/clusters", "createFederationCluster", "Register federation cluster", "P2 Enterprise", "admin", "P2", "", `        '201': { description: Created }`, ""},
	{"delete", "/federation/clusters/{id}", "deleteFederationCluster", "Remove federation cluster", "P2 Enterprise", "admin", "P2", "", `        '204': { description: Deleted }`, "    - name: id\n      in: path\n      required: true\n      schema: { type: string }\n"},
	{"get", "/cluster/status", "clusterStatus", "Cluster node status", "P2 Enterprise", "admin", "P2", "", `        '200': { description: Status }`, ""},
	{"post", "/sts/assume-role", "assumeRole", "STS assume-role (MVP)", "P2 Enterprise", "any", "P2", "", `        '200': { description: Temporary credentials }`, ""},
	{"put", "/buckets/{bucket}/object-retention", "putObjectRetention", "Set object retention", "P2 Enterprise", "any", "P2", "", `        '200': { description: Updated }`, "    - $ref: '#/components/parameters/BucketName'\n"},
	{"get", "/buckets/{bucket}/object-retention", "getObjectRetention", "Get object retention", "P2 Enterprise", "any", "P2", "", `        '200': { description: Retention }`, "    - $ref: '#/components/parameters/BucketName'\n"},
}

func writePath(b *strings.Builder, path string, ops []opDef, full bool) {
	fmt.Fprintf(b, "  %s:\n", path)
	for _, op := range ops {
		writeMethod(b, op, full)
	}
}

func writeMethod(b *strings.Builder, op opDef, full bool) {
	fmt.Fprintf(b, "    %s:\n", op.method)
	fmt.Fprintf(b, "      operationId: %s\n", op.id)
	fmt.Fprintf(b, "      summary: %s\n", op.summary)
	fmt.Fprintf(b, "      tags: [%s]\n", resolveTag(op, full))
	if !full {
		b.WriteString("      x-community: true\n")
	}
	if op.security == "public" || op.security == "" {
		b.WriteString("      security: []\n")
	} else if op.security == "jwt" && full {
		b.WriteString("      security:\n        - BearerJWT: []\n        - BearerAPIToken: []\n")
	} else if op.security == "admin" && full {
		b.WriteString("      security:\n        - BearerJWT: []\n        - BearerAPIToken: []\n")
		b.WriteString("      x-required-role: administrator\n")
	} else {
		b.WriteString("      security:\n        - BearerAPIToken: []\n")
		if full {
			b.WriteString("        - BearerJWT: []\n")
		}
	}
	if op.params != "" {
		b.WriteString("      parameters:\n")
		for _, line := range strings.Split(strings.TrimSuffix(op.params, "\n"), "\n") {
			b.WriteString("      " + line + "\n")
		}
	}
	if op.bodySchema != "" {
		fmt.Fprintf(b, "      requestBody:\n        required: true\n        content:\n          application/json:\n            schema: { $ref: '#/components/schemas/%s' }\n", op.bodySchema)
	}
	b.WriteString("      responses:\n")
	b.WriteString(op.responses)
	if !strings.HasSuffix(op.responses, "\n") {
		b.WriteString("\n")
	}
	if op.security != "public" && op.security != "" {
		if !strings.Contains(op.responses, "'401'") {
			b.WriteString("        '401': { $ref: '#/components/responses/Unauthorized' }\n")
		}
		if !strings.Contains(op.responses, "'403'") {
			b.WriteString("        '403': { $ref: '#/components/responses/Forbidden' }\n")
		}
	}
	if op.bodySchema != "" && !strings.Contains(op.responses, "'400'") {
		b.WriteString("        '400': { $ref: '#/components/responses/BadRequest' }\n")
	}
	if strings.Contains(op.path, "{") && op.method != "post" && !strings.Contains(op.responses, "'404'") {
		if op.security != "public" && op.security != "" {
			b.WriteString("        '404': { $ref: '#/components/responses/NotFound' }\n")
		}
	}
	b.WriteString("\n")
}

func resolveTag(op opDef, full bool) string {
	if !full {
		return communityTag(op)
	}
	switch {
	case op.path == "/health" || op.path == "/healthz":
		return "Health"
	case strings.HasPrefix(op.path, "/admin/login") || strings.HasPrefix(op.path, "/mfa/") || strings.HasPrefix(op.path, "/auth/"):
		return "Auth"
	case strings.HasPrefix(op.path, "/me"):
		return "Profile"
	case op.path == "/usage":
		return "Usage"
	case strings.HasPrefix(op.path, "/tokens"):
		return "Tokens"
	case strings.HasPrefix(op.path, "/keys") || op.path == "/presign":
		return "Keys"
	case strings.Contains(op.path, "/share") || strings.HasPrefix(op.path, "/public/"):
		return "Shares"
	case strings.HasPrefix(op.path, "/users") || strings.HasPrefix(op.path, "/activity") ||
		strings.HasPrefix(op.path, "/settings/"):
		return "Admin"
	case strings.HasPrefix(op.path, "/webhooks"), strings.HasPrefix(op.path, "/hooks/"):
		return "Webhooks"
	case strings.HasPrefix(op.path, "/tenants"):
		return "Tenants"
	case strings.HasPrefix(op.path, "/gateway"):
		return "Gateway"
	case strings.HasPrefix(op.path, "/federation") || op.path == "/cluster/status":
		return "Federation"
	case strings.HasPrefix(op.path, "/settings/ldap") || strings.HasPrefix(op.path, "/mfa/") || op.path == "/sts/assume-role":
		return "Enterprise"
	case op.path == "/search" || strings.HasPrefix(op.path, "/favorites") || strings.HasPrefix(op.path, "/trash") ||
		strings.Contains(op.path, "/tags") || strings.Contains(op.path, "/object-meta"):
		return "Search"
	case strings.Contains(op.path, "/lifecycle") || strings.Contains(op.path, "/legal-hold") || strings.Contains(op.path, "/object-retention"):
		return "Lifecycle"
	case strings.Contains(op.path, "/policy"):
		return "Admin"
	default:
		return "Buckets"
	}
}

func communityTag(op opDef) string {
	switch {
	case op.path == "/health":
		return "Health"
	case strings.HasPrefix(op.path, "/me"):
		return "Profile"
	case op.path == "/usage":
		return "Usage"
	case strings.HasPrefix(op.path, "/tokens"):
		return "Tokens"
	case strings.HasPrefix(op.path, "/keys") || op.path == "/presign":
		return "Keys"
	case strings.Contains(op.path, "/share") || strings.HasPrefix(op.path, "/public/"):
		return "Shares"
	case op.path == "/search" || strings.HasPrefix(op.path, "/favorites") || strings.HasPrefix(op.path, "/trash") ||
		strings.Contains(op.path, "/tags") || strings.Contains(op.path, "/object-meta"):
		return "Search"
	case strings.Contains(op.path, "/lifecycle") || strings.Contains(op.path, "/legal-hold") || strings.Contains(op.path, "/object-retention"):
		return "Lifecycle"
	default:
		return "Buckets"
	}
}
