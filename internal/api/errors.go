package api

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// API error codes. Legacy `error` strings stay as they were in 1.6.0.
// New codes are additive; do not rename a code for an existing case.
const (
	codeAuthUnauthorized       = "auth.unauthorized"
	codeAuthInvalidCredentials = "auth.invalid_credentials"
	codeAuthSessionExpired     = "auth.session_expired"
	codeAuthStepUpRequired     = "auth.step_up_required"
	codeAuthMFASetupRequired   = "auth.mfa_setup_required"
	codeAuthMFARequired        = "auth.mfa_required"
	codeAuthMFATokenInvalid    = "auth.mfa_token_invalid"
	codeAccessForbidden        = "access.forbidden"
	codeAccessQuotaExceeded    = "access.quota_exceeded"
	codeSharingPolicyDenied    = "sharing.policy_denied"
	codeResourceNotFound       = "resource.not_found"
	codeResourceConflict       = "resource.conflict"
	codeResourceGone           = "resource.gone"
	codeMultipartUploadMissing = "multipart.upload_not_found"
	codeRequestInvalid         = "request.invalid"
	codeRateLimited            = "rate_limited"
	codeSetupRequired          = "setup.required"
	codeSetupPasswordChange    = "setup.password_change_required"
	codeInternal               = "internal"
)

// enrichAPIError adds code, message, details, and request_id to an error
// object. The legacy error string is left unchanged. The caller's map is not
// mutated.
func enrichAPIError(w http.ResponseWriter, status int, body map[string]any) map[string]any {
	out := make(map[string]any, len(body)+4)
	for k, v := range body {
		out[k] = v
	}
	legacy, _ := out["error"].(string)
	if _, ok := out["code"].(string); !ok || out["code"] == "" {
		out["code"] = errorCode(status, legacy, out)
	}
	if _, ok := out["message"].(string); !ok || strings.TrimSpace(out["message"].(string)) == "" {
		out["message"] = errorMessage(legacy, out["code"].(string))
	}
	if _, ok := out["details"].(map[string]any); !ok {
		out["details"] = errorDetails(out)
	}
	if _, ok := out["request_id"].(string); !ok || out["request_id"] == "" {
		out["request_id"] = requestID(w)
	}
	return out
}

func requestID(w http.ResponseWriter) string {
	id := strings.TrimSpace(w.Header().Get("X-Request-Id"))
	if id == "" {
		id = randomHex(8)
		w.Header().Set("X-Request-Id", id)
	}
	return id
}

func assignRequestID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if id == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n") {
		id = randomHex(8)
	}
	w.Header().Set("X-Request-Id", id)
}

func errorCode(status int, legacy string, body map[string]any) string {
	switch legacy {
	case "unauthorized":
		return codeAuthUnauthorized
	case "invalid credentials":
		return codeAuthInvalidCredentials
	case "session expired":
		return codeAuthSessionExpired
	case "step_up_required":
		return codeAuthStepUpRequired
	case "mfa_setup_required":
		return codeAuthMFASetupRequired
	case "mfa_required":
		return codeAuthMFARequired
	case "mfa_token_invalid", "invalid mfa token", "invalid or expired mfa token":
		return codeAuthMFATokenInvalid
	case "forbidden":
		return codeAccessForbidden
	case "quota exceeded":
		return codeAccessQuotaExceeded
	case "sharing_disabled", "password_required", "max_share_ttl", "presign_disabled", "presign_blocked":
		return codeSharingPolicyDenied
	case "upload not found":
		return codeMultipartUploadMissing
	case "not found", "object not found", "user not found", "tenant not found", "share not found":
		return codeResourceNotFound
	case "rate limit exceeded":
		return codeRateLimited
	case "setup_required":
		return codeSetupRequired
	case "password_change_required":
		return codeSetupPasswordChange
	}
	if body["mfa_setup_required"] == true {
		return codeAuthMFASetupRequired
	}
	switch {
	case status == http.StatusUnauthorized:
		return codeAuthUnauthorized
	case status == http.StatusForbidden:
		return codeAccessForbidden
	case status == http.StatusNotFound:
		return codeResourceNotFound
	case status == http.StatusConflict:
		return codeResourceConflict
	case status == http.StatusGone:
		return codeResourceGone
	case status == http.StatusTooManyRequests:
		return codeRateLimited
	case status >= 500:
		return codeInternal
	default:
		return codeRequestInvalid
	}
}

func errorMessage(legacy, code string) string {
	if strings.TrimSpace(legacy) != "" {
		return sentence(legacy)
	}
	return sentence(strings.ReplaceAll(code, ".", " "))
}

func sentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "_", " "))
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 1 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func errorDetails(body map[string]any) map[string]any {
	details := map[string]any{}
	for k, v := range body {
		switch k {
		case "error", "code", "message", "details", "request_id":
			continue
		default:
			details[k] = v
		}
	}
	return details
}
