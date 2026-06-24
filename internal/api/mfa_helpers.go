package api

import (
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) jwtSecret() string {
	if s.cfg.JWTSecret != "" {
		return s.cfg.JWTSecret
	}
	return "datasafe-jwt-secret"
}

func (s *Server) userTOTPSecret(user metadata.UserRecord) (string, error) {
	return auth.DecryptTOTPSecret(s.jwtSecret(), user.TOTPSecret)
}

func (s *Server) validateUserMFACode(user *metadata.UserRecord, code string) bool {
	code = trimCode(code)
	if code == "" {
		return false
	}
	secret, err := s.userTOTPSecret(*user)
	if err == nil && secret != "" && auth.ValidateTOTP(secret, code) {
		return true
	}
	if updated, ok := auth.ConsumeRecoveryCode(user.RecoveryCodes, code); ok {
		user.RecoveryCodes = updated
		return true
	}
	return false
}

func trimCode(code string) string {
	for len(code) > 0 && (code[0] == ' ' || code[0] == '\t') {
		code = code[1:]
	}
	for len(code) > 0 && (code[len(code)-1] == ' ' || code[len(code)-1] == '\t') {
		code = code[:len(code)-1]
	}
	return code
}
