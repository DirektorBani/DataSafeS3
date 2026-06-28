package storage

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalidKey is returned when an object key fails validation.
var ErrInvalidKey = errors.New("invalid object key")

const maxObjectKeyLen = 1024

// ValidateObjectKey rejects unsafe S3 object keys before filesystem mapping.
func ValidateObjectKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: empty object key", ErrInvalidKey)
	}
	if len(key) > maxObjectKeyLen {
		return fmt.Errorf("object key exceeds %d bytes", maxObjectKeyLen)
	}
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("object key must not start with /")
	}
	if strings.Contains(key, "\\") {
		return fmt.Errorf("object key must not contain backslash")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("object key must not contain ..")
	}
	for _, r := range key {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("object key contains invalid character")
		}
	}
	return nil
}
