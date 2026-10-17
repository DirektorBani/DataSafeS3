package main

import (
	"net/http"
	"net/url"
	"strings"
)

// shareCreate posts a share link. On 1.6, when the bucket policy is readable,
// a disabled link, a missing required password, or a TTL past the cap is
// refused here and the server is not asked to create the link. A 1.5.0
// server has no sharing.policy feature; the request is posted as entered
// and the server decides. The password is the caller's share_password. It
// is not stored and is not returned.
func (a *App) shareCreate(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	key := strParam(params, "key")
	if key == "" {
		return apiResult{Status: http.StatusBadRequest, Code: "request.invalid", Message: "request.invalid"}, nil
	}
	expires := intParam(params, "expires_in_sec")
	password := strParam(params, "share_password")
	if blocked, err := a.policyBlock(params, path, expires, password, false); blocked != nil || err != nil {
		if blocked != nil {
			return *blocked, err
		}
		return apiResult{}, err
	}
	body := map[string]any{
		"key":            key,
		"expires_in_sec": expires,
		"max_downloads":  intParam(params, "max_downloads"),
	}
	if password != "" {
		body["password"] = password
	}
	return a.browseCall(params, http.MethodPost, path+"/shares", nil, body)
}

func (a *App) shareList(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	if key := strParam(params, "key"); key != "" {
		query.Set("key", key)
	}
	return a.browseCall(params, http.MethodGet, path+"/shares", query, nil)
}

func (a *App) shareRevoke(params map[string]any) (apiResult, error) {
	id := strParam(params, "share_id")
	if id == "" {
		return apiResult{Status: http.StatusBadRequest, Code: "request.invalid", Message: "request.invalid"}, nil
	}
	return a.browseCall(params, http.MethodDelete, "/api/v1/shares/"+url.PathEscape(id), nil, nil)
}

func (a *App) shareRevokeAll(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodPost, path+"/shares/revoke-all", nil, map[string]any{})
}

func (a *App) sharePolicy(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodGet, path+"/sharing", nil, nil)
}

// presignCreate asks for a presigned URL. The same policy read as a share
// link applies on 1.6. A presigned URL cannot be revoked; the window says so.
func (a *App) presignCreate(params map[string]any) (apiResult, error) {
	bucket := strParam(params, "bucket")
	key := strParam(params, "key")
	if bucket == "" || key == "" {
		return apiResult{Status: http.StatusBadRequest, Code: "request.invalid", Message: "request.invalid"}, nil
	}
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	expires := intParam(params, "expires_seconds")
	if blocked, err := a.policyBlock(params, path, expires, "", true); blocked != nil || err != nil {
		if blocked != nil {
			return *blocked, err
		}
		return apiResult{}, err
	}
	method := strings.ToUpper(strParam(params, "method"))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut:
	default:
		method = http.MethodGet
	}
	return a.browseCall(params, http.MethodPost, "/api/v1/presign", nil, map[string]any{
		"method":          method,
		"bucket":          bucket,
		"key":             key,
		"expires_seconds": expires,
	})
}

// policyBlock reads the bucket sharing policy when the server advertises
// sharing.policy and the read succeeds. Any other outcome leaves the
// decision to the server.
func (a *App) policyBlock(params map[string]any, bucketPath string, expires int, password string, presign bool) (*apiResult, error) {
	profile, err := a.profileFromParams(params)
	if err != nil {
		return nil, err
	}
	if !featureOn(profile, "sharing.policy") {
		return nil, nil
	}
	policy, err := a.browseCall(params, http.MethodGet, bucketPath+"/sharing", nil, nil)
	if err != nil || policy.Status != http.StatusOK {
		return nil, err
	}
	return sharePolicyBlock(policy.Body, expires, password, presign), nil
}

func featureOn(profile Profile, name string) bool {
	if profile.Server == nil || profile.Server.Features == nil {
		return false
	}
	return profile.Server.Features[name] > 0
}

// sharePolicyBlock is the client-side cap. A nil result means the request
// may be posted. max TTL of 0, or a missing field, is not a cap. A negative
// TTL is the server's "no cap" and is not a cap either.
func sharePolicyBlock(body map[string]any, expires int, password string, presign bool) *apiResult {
	effective, _ := body["effective"].(map[string]any)
	if effective == nil {
		return nil
	}
	if allowed, ok := boolField(effective, "share_links_allowed"); ok && !allowed {
		return &apiResult{
			Status:  http.StatusForbidden,
			Code:    "sharing_disabled",
			Message: "sharing_disabled",
			Body:    map[string]any{"error": "sharing_disabled"},
		}
	}
	if presign {
		if allowed, ok := boolField(effective, "presign_allowed"); ok && !allowed {
			return &apiResult{
				Status:  http.StatusForbidden,
				Code:    "presign_disabled",
				Message: "presign_disabled",
				Body:    map[string]any{"error": "presign_disabled"},
			}
		}
		if max, ok := int64Field(effective, "max_presign_ttl_sec"); ok && max > 0 && (expires <= 0 || int64(expires) > max) {
			return ttlResult("max_presign_ttl", max)
		}
		return nil
	}
	if required, ok := boolField(effective, "share_password_required"); ok && required && strings.TrimSpace(password) == "" {
		return &apiResult{
			Status:  http.StatusBadRequest,
			Code:    "password_required",
			Message: "password_required",
			Body:    map[string]any{"error": "password_required"},
		}
	}
	if max, ok := int64Field(effective, "max_share_ttl_sec"); ok && max > 0 && (expires <= 0 || int64(expires) > max) {
		return ttlResult("max_share_ttl", max)
	}
	return nil
}

func ttlResult(code string, max int64) *apiResult {
	return &apiResult{
		Status:  http.StatusBadRequest,
		Code:    code,
		Message: code,
		Body:    map[string]any{"error": code, code + "_sec": max},
	}
}

func boolField(obj map[string]any, key string) (bool, bool) {
	value, ok := obj[key].(bool)
	return value, ok
}

func int64Field(obj map[string]any, key string) (int64, bool) {
	switch value := obj[key].(type) {
	case float64:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	default:
		return 0, false
	}
}
