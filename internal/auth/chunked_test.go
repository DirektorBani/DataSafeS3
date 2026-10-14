package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
)

func TestChecksumKnownAnswers(t *testing.T) {
	payload := []byte("123456789")
	// CRC64NVME("123456789") is the AWS check value. The others are the
	// ISO-HDLC, Castagnoli, SHA-1, and SHA-256 check values of the same bytes.
	cases := []struct {
		name string
		hex  string
	}{
		{"x-amz-checksum-crc64nvme", "ae8b14860a799888"},
		{"x-amz-checksum-crc32", "cbf43926"},
		{"x-amz-checksum-crc32c", "e3069283"},
		{"x-amz-checksum-sha1", "f7c3bc1d808e04732adf679965ccc34ca7ae3441"},
		{"x-amz-checksum-sha256", "15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			got, err := readUnsignedTrailer(t, payload, tc.name, base64.StdEncoding.EncodeToString(raw))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("body %q", got)
			}
		})
	}
	_, err := readUnsignedTrailer(t, payload, "x-amz-checksum-crc64nvme", base64.StdEncoding.EncodeToString([]byte("not-the-sum!!")))
	if !errors.Is(err, ErrBadDigest) {
		t.Fatalf("bad crc64: %v", err)
	}
}

func readUnsignedTrailer(t *testing.T, payload []byte, name, b64 string) ([]byte, error) {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString(strconv.FormatInt(int64(len(payload)), 16))
	buf.WriteString("\r\n")
	buf.Write(payload)
	buf.WriteString("\r\n0\r\n")
	buf.WriteString(name)
	buf.WriteByte(':')
	buf.WriteString(b64)
	buf.WriteString("\r\n\r\n")
	req, err := http.NewRequest(http.MethodPut, "http://example/bucket/key", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Amz-Content-Sha256", streamingUnsignedTrailer)
	req.Header.Set("X-Amz-Decoded-Content-Length", strconv.Itoa(len(payload)))
	r, n, err := UnwrapAWSChunked(req, "")
	if err != nil {
		return nil, err
	}
	if n != int64(len(payload)) {
		t.Fatalf("decoded length %d", n)
	}
	return io.ReadAll(r)
}
