package api

import (
	"os"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) jwtSecret() string {
	if s.cfg.JWTSecret != "" {
		return s.cfg.JWTSecret
	}
	return "datasafe-jwt-secret"
}

func (s *Server) mfaEncryptionKey() string {
	if k := os.Getenv("STORAGE_MFA_ENCRYPTION_KEY"); k != "" {
		return k
	}
	return s.jwtSecret()
}

func (s *Server) userTOTPSecret(user metadata.UserRecord) (string, error) {
	return auth.DecryptTOTPSecretWithFallback(s.mfaEncryptionKey(), s.jwtSecret(), user.TOTPSecret)
}

func (s *Server) validateUserMFACode(user *metadata.UserRecord, code string) bool {
	code = trimCode(code)
	if code == "" {
		return false
	}
	secret, err := s.userTOTPSecret(*user)
	if err == nil && secret != "" {
		if step, ok := auth.TOTPStep(secret, code, time.Now()); ok {
			claimed, cerr := s.meta.ClaimTOTPStep(user.ID, step)
			if cerr != nil || !claimed {
				return false
			}
			user.TOTPLastStep = step
			return true
		}
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
