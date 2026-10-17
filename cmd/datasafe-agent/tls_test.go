package main

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSelfSignedTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	strict, err := newClientConfig(srv.URL, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strict.do(http.MethodGet, "/", nil); err == nil {
		t.Fatal("self-signed certificate was accepted")
	}

	loose, err := newClientConfig(srv.URL, true, "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := loose.do(http.MethodGet, "/", nil)
	if err != nil || resp.Status != http.StatusNoContent {
		t.Fatalf("insecure_dev status %d err %v", resp.Status, err)
	}

	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, block, 0o600); err != nil {
		t.Fatal(err)
	}
	trusted, err := newClientConfig(srv.URL, false, caPath)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = trusted.do(http.MethodGet, "/", nil)
	if err != nil || resp.Status != http.StatusNoContent {
		t.Fatalf("ca file status %d err %v", resp.Status, err)
	}

	if _, err := newClient("http://files.example", false); err == nil {
		t.Fatal("plain http to a remote host was accepted")
	}
}

func TestCertificateFailureIsNotUnreachable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := newClientConfig(srv.URL, false, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.do(http.MethodGet, "/healthz", nil)
	if netReason(err) != "certificate" {
		t.Fatalf("reason %q err %v", netReason(err), err)
	}
	var card ServerCard
	classifyHealth(&card, healthInfo{}, false, netReason(err))
	if card.Warning != "the server certificate could not be verified" || card.Profile != "tls" || !card.Rejected {
		t.Fatalf("%+v", card)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	down, err := newClient("http://"+addr, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = down.do(http.MethodGet, "/healthz", nil)
	if netReason(err) != "unreachable" {
		t.Fatalf("reason %q err %v", netReason(err), err)
	}
	card = ServerCard{}
	classifyHealth(&card, healthInfo{}, false, netReason(err))
	if card.Warning != "server is unreachable" || card.Profile != "unreachable" {
		t.Fatalf("%+v", card)
	}
}
