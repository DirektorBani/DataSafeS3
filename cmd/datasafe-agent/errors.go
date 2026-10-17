package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// apiError is the desktop view of a server error (spec 4.6.7).
type apiError struct {
	Status     int    `json:"status"`
	Code       string `json:"code"`
	Legacy     string `json:"legacy,omitempty"`
	Message    string `json:"message,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
	StepUp     bool   `json:"step_up,omitempty"`
	MFASetup   bool   `json:"mfa_setup,omitempty"`
}

func classifyError(status int, body []byte, header http.Header) apiError {
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	legacy, _ := payload["error"].(string)
	message, _ := payload["message"].(string)
	code, _ := payload["code"].(string)
	if _, ok := payload["mfa_setup_required"]; ok && code == "" {
		legacy = "mfa_setup_required"
	}
	if code == "" {
		code = legacyCode(status, legacy)
	}
	out := apiError{
		Status:  status,
		Code:    code,
		Legacy:  legacy,
		Message: message,
		StepUp: code == "auth.step_up_required" || legacy == "step_up_required" ||
			code == "auth.mfa_token_invalid" || legacy == "mfa_token_invalid" ||
			code == "auth.invalid_code" || legacy == "invalid code",
		MFASetup: code == "auth.mfa_setup_required" || legacy == "mfa_setup_required" ||
			code == "auth.mfa_required" || legacy == "mfa_required",
	}
	if status == http.StatusTooManyRequests {
		out.RetryAfter = 60
		if header != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && n > 0 {
				out.RetryAfter = n
			}
		}
	}
	if message == "" {
		out.Message = legacy
	}
	return out
}

func legacyCode(status int, legacy string) string {
	switch legacy {
	case "unauthorized":
		return "auth.unauthorized"
	case "session expired":
		return "auth.session_expired"
	case "step_up_required":
		return "auth.step_up_required"
	case "mfa_token_invalid":
		return "auth.mfa_token_invalid"
	case "invalid code":
		return "auth.invalid_code"
	case "mfa_setup_required":
		return "auth.mfa_setup_required"
	case "mfa_required":
		return "auth.mfa_required"
	case "forbidden":
		return "access.forbidden"
	case "not found":
		return "resource.not_found"
	case "upload not found":
		return "multipart.upload_not_found"
	case "rate limit exceeded":
		return "rate_limited"
	}
	switch status {
	case http.StatusUnauthorized:
		return "auth.unauthorized"
	case http.StatusForbidden:
		return "access.forbidden"
	case http.StatusNotFound:
		return "resource.not_found"
	case http.StatusBadRequest:
		return "request.invalid"
	case http.StatusConflict:
		return "resource.conflict"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		if status >= 500 {
			return "internal"
		}
		return "request.invalid"
	}
}

func reauthCode(err apiError) bool {
	if err.StepUp || err.MFASetup {
		return false
	}
	return err.Status == http.StatusUnauthorized
}
