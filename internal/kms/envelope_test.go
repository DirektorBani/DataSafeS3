package kms

import (
	"bytes"
	"io"
	"testing"

	"github.com/DirektorBani/datasafe/internal/storage"
)

func TestFormat1RoundTrip(t *testing.T) {
	dek := bytes.Repeat([]byte{7}, 32)
	sizes := []int{0, 1, 17, DefaultChunk - 1, DefaultChunk, DefaultChunk + 3, DefaultChunk*2 + 10}
	for _, n := range sizes {
		plain := bytes.Repeat([]byte{byte(n + 1)}, n)
		r, err := EncryptReader(dek, bytes.NewReader(plain), DefaultChunk)
		if err != nil {
			t.Fatal(err)
		}
		ct, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if n >= 16 && bytes.Contains(ct, plain) {
			t.Fatalf("size %d: ciphertext contains plaintext", n)
		}
		out, err := DecryptReader(dek, bytes.NewReader(ct))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(out)
		if err != nil {
			t.Fatalf("size %d: %v", n, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("size %d: got %d bytes", n, len(got))
		}
	}
}

func TestFormat0MatchesSSE15(t *testing.T) {
	const master = "test-master-key-32chars-minimum!!"
	plain := []byte("object written by 1.5.0")
	old, err := storage.NewSSECipher(master)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := old.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	local, err := NewLocal(master)
	if err != nil || local == nil {
		t.Fatal(err)
	}
	got, err := local.DecryptFormat0(ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("format 0 mismatch %q", got)
	}
	again, err := local.EncryptFormat0(plain)
	if err != nil {
		t.Fatal(err)
	}
	back, err := old.Decrypt(again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, plain) {
		t.Fatal("1.5.0 cipher cannot read format 0 from 1.6.0")
	}
}

func TestWrapRoundTrip(t *testing.T) {
	local, err := NewLocal("master")
	if err != nil {
		t.Fatal(err)
	}
	dek, err := NewDEK()
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ver, err := local.Wrap(t.Context(), "datasafe-t-b", dek)
	if err != nil {
		t.Fatal(err)
	}
	if ver != 1 {
		t.Fatalf("version %d", ver)
	}
	out, err := local.Unwrap(t.Context(), "datasafe-t-b", ver, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, dek) {
		t.Fatal("dek mismatch")
	}
	if bytes.Contains(wrapped, dek) {
		t.Fatal("wrapped dek contains the clear dek")
	}
}

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("same")
	b := Fingerprint("same")
	c := Fingerprint("other")
	if a == "" || a != b || a == c {
		t.Fatalf("fingerprint %s %s %s", a, b, c)
	}
	if Fingerprint("") != "" {
		t.Fatal("empty master has a fingerprint")
	}
}
