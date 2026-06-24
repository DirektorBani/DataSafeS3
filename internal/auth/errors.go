package auth

import "errors"

var (
	ErrMissingAuth         = errors.New("missing authorization")
	ErrInvalidAuth         = errors.New("invalid authorization")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrSignatureMismatch   = errors.New("signature does not match")
	ErrExpired             = errors.New("presigned url expired")
)
