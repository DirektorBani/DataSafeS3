package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestMFAEnrollFlow(t *testing.T) {
	srv := testServer(t)
	token := loginToken(t, srv, "admin", "admin")

	enrollReq := authReq(http.MethodPost, "/api/v1/mfa/enroll", token, nil)
	enrollW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(enrollW, enrollReq)
	if enrollW.Code != http.StatusOK {
		t.Fatalf("enroll: %d %s", enrollW.Code, enrollW.Body.String())
	}
	var enrollResp struct {
		Secret     string `json:"secret"`
		OtpauthURI string `json:"otpauth_uri"`
		QRCode     string `json:"qr_code"`
	}
	if err := json.Unmarshal(enrollW.Body.Bytes(), &enrollResp); err != nil {
		t.Fatal(err)
	}
	if enrollResp.Secret == "" || enrollResp.OtpauthURI == "" || enrollResp.QRCode == "" {
		t.Fatalf("missing enrollment fields: %+v", enrollResp)
	}

	code, err := auth.GenerateTOTPCode(enrollResp.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := json.Marshal(map[string]string{"code": code})
	verifyReq := authReq(http.MethodPost, "/api/v1/mfa/verify-enroll", token, verifyBody)
	verifyW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(verifyW, verifyReq)
	if verifyW.Code != http.StatusOK {
		t.Fatalf("verify-enroll: %d %s", verifyW.Code, verifyW.Body.String())
	}
	var verifyResp struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(verifyW.Body.Bytes(), &verifyResp); err != nil {
		t.Fatal(err)
	}
	if len(verifyResp.RecoveryCodes) != 10 {
		t.Fatalf("expected 10 recovery codes, got %d", len(verifyResp.RecoveryCodes))
	}
}

func TestMFALoginFlow(t *testing.T) {
	srv := testServer(t)
	token := loginToken(t, srv, "admin", "admin")

	enrollReq := authReq(http.MethodPost, "/api/v1/mfa/enroll", token, nil)
	enrollW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(enrollW, enrollReq)
	var enrollResp struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(enrollW.Body.Bytes(), &enrollResp); err != nil {
		t.Fatal(err)
	}
	code, err := auth.GenerateTOTPCode(enrollResp.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := json.Marshal(map[string]string{"code": code})
	verifyReq := authReq(http.MethodPost, "/api/v1/mfa/verify-enroll", token, verifyBody)
	verifyW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(verifyW, verifyReq)
	if verifyW.Code != http.StatusOK {
		t.Fatalf("verify-enroll: %d %s", verifyW.Code, verifyW.Body.String())
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login step1: %d %s", loginW.Code, loginW.Body.String())
	}
	var loginResp struct {
		MFARequired bool   `json:"mfa_required"`
		MFAToken    string `json:"mfa_token"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(loginW.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if !loginResp.MFARequired || loginResp.MFAToken == "" || loginResp.Token != "" {
		t.Fatalf("expected mfa_required + mfa_token, got %+v", loginResp)
	}

	mfaCode, err := auth.GenerateTOTPCode(enrollResp.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	mfaBody, _ := json.Marshal(map[string]string{"mfa_token": loginResp.MFAToken, "totp_code": mfaCode})
	mfaReq := httptest.NewRequest(http.MethodPost, "/api/v1/mfa/login", bytes.NewReader(mfaBody))
	mfaReq.Header.Set("Content-Type", "application/json")
	mfaW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mfaW, mfaReq)
	if mfaW.Code != http.StatusOK {
		t.Fatalf("mfa login: %d %s", mfaW.Code, mfaW.Body.String())
	}
	var mfaResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(mfaW.Body.Bytes(), &mfaResp); err != nil {
		t.Fatal(err)
	}
	if mfaResp.Token == "" {
		t.Fatal("expected jwt from mfa login")
	}
}
