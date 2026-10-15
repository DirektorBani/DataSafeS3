package kms

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestAuthenticateFormat0Streams(t *testing.T) {
	local, err := NewLocal("legacy-key")
	if err != nil || local == nil {
		t.Fatal(err)
	}
	cases := [][]byte{
		nil,
		[]byte("from-15"),
		bytes.Repeat([]byte("plain-from-15\n"), 1000),
		bytes.Repeat([]byte{0}, 8192),
		func() []byte {
			b := make([]byte, 1<<20)
			_, _ = rand.Read(b)
			return b
		}(),
	}
	for _, plain := range cases {
		ct, err := local.EncryptFormat0(plain)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := local.AuthenticateFormat0(bytes.NewReader(ct))
		if err != nil || !ok {
			t.Fatalf("auth len %d ok %v %v", len(plain), ok, err)
		}
		ct[len(ct)-1] ^= 0xff
		ok, err = local.AuthenticateFormat0(bytes.NewReader(ct))
		if err != nil || ok {
			t.Fatalf("tampered len %d ok %v %v", len(plain), ok, err)
		}
	}
	raw := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0x3c}, 4096)...)
	ok, err := local.AuthenticateFormat0(bytes.NewReader(raw))
	if err != nil || ok {
		t.Fatalf("jpeg-like ok %v %v", ok, err)
	}
	short := []byte("tiny")
	ok, err = local.AuthenticateFormat0(bytes.NewReader(short))
	if err != nil || ok {
		t.Fatalf("short ok %v %v", ok, err)
	}
	dsse := append([]byte{'D', 'S', 'S', 'E', 1}, bytes.Repeat([]byte{0}, 32)...)
	_, err = local.AuthenticateFormat0(bytes.NewReader(dsse))
	if err != ErrFormat1Header {
		t.Fatalf("dsse %v", err)
	}
	// A reader that yields one byte at a time still authenticates.
	ct, err := local.EncryptFormat0([]byte("chunked-read"))
	if err != nil {
		t.Fatal(err)
	}
	ok, err = local.AuthenticateFormat0(&oneByteReader{b: ct})
	if err != nil || !ok {
		t.Fatalf("one-byte %v %v", ok, err)
	}
}

type oneByteReader struct {
	b []byte
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.b[0]
	r.b = r.b[1:]
	return 1, nil
}
