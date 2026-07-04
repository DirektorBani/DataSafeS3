package pki

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCA_andFingerprint(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	fp1, err := m.CAFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if len(fp1) != 64 {
		t.Fatalf("fingerprint len %d", len(fp1))
	}
	fp2, err := m.CAFingerprint()
	if err != nil || fp1 != fp2 {
		t.Fatalf("fingerprint not stable: %s vs %s", fp1, fp2)
	}
	if _, err := os.Stat(filepath.Join(dir, caCertFile)); err != nil {
		t.Fatal(err)
	}
}

func TestSignCSR(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	csrPEM, _, err := m.GenerateCSR("test-cluster")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, serial, nb, na, err := m.SignCSR(csrPEM, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if serial == "" || len(certPEM) == 0 {
		t.Fatal("empty cert output")
	}
	if !na.After(nb) {
		t.Fatal("bad validity window")
	}
}

func TestVerifyPeerCACert_rejectsNonCA(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	csrPEM, _, err := m.GenerateCSR("leaf")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _, _, _, err := m.SignCSR(csrPEM, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyPeerCACert(certPEM); err == nil {
		t.Fatal("expected non-CA rejection")
	}
}

func TestVerifyPeerCACert_acceptsCA(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	pemBytes, err := m.CACertPEM()
	if err != nil {
		t.Fatal(err)
	}
	fp, err := m.VerifyPeerCACert(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if fp == "" {
		t.Fatal("empty fingerprint")
	}
}

func TestSPKIFingerprint_deterministic(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	pemBytes, _ := m.CACertPEM()
	cert, _ := ParseCertificatePEM(pemBytes)
	a := SPKIFingerprint(cert)
	b := SPKIFingerprint(cert)
	if a != b {
		t.Fatalf("not deterministic")
	}
}
