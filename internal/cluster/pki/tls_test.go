package pki

import "testing"

func TestCheckSerialRevoked_rejectsRevoked(t *testing.T) {
	revoked := revokedSerialSet([]string{"abc123", "DEADbeef"})
	if err := CheckSerialRevoked("abc123", revoked); err != ErrRevokedCertSerial {
		t.Fatalf("expected revoked error, got %v", err)
	}
	if err := CheckSerialRevoked("deadbeef", revoked); err != ErrRevokedCertSerial {
		t.Fatalf("expected case-insensitive revoke, got %v", err)
	}
	if err := CheckSerialRevoked("okserial", revoked); err != nil {
		t.Fatalf("expected active serial ok, got %v", err)
	}
}
