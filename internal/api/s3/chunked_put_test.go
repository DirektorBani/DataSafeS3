package s3_test

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestBucketLocationIsNotAListing(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "loc")
	put(t, ts.URL, creds, "/loc/a.txt", "a")
	body := getRaw(t, ts.URL, creds, "/loc?location")
	if !strings.Contains(body, "LocationConstraint") || strings.Contains(body, "<Key>") {
		t.Fatalf("location %s", body)
	}
}

func TestSignedChunkedPutStoresDecodedBytes(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "chk")

	payload := []byte("hello-aws")
	resp := doChunked(t, ts.URL+"/chk/nine.bin", creds, payload, "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put %d", resp.StatusCode)
	}
	sum := md5.Sum(payload)
	wantETag := `"` + hex.EncodeToString(sum[:]) + `"`
	if resp.Header.Get("ETag") != wantETag {
		t.Fatalf("etag %s want %s", resp.Header.Get("ETag"), wantETag)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/chk/nine.bin", nil)
	signRequest(t, req, creds, "us-east-1")
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	body, _ := io.ReadAll(got.Body)
	if got.StatusCode != http.StatusOK || !bytes.Equal(body, payload) || got.ContentLength != int64(len(payload)) {
		t.Fatalf("stored %d %q len %d", got.StatusCode, body, got.ContentLength)
	}
}

func TestSignedChunkedUploadPart(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "cmp")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/cmp/big.bin?uploads", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var initResp struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(raw, &initResp); err != nil || initResp.UploadID == "" {
		t.Fatalf("upload id: %v %s", err, raw)
	}
	payload := []byte("hello-aws")
	part := doChunked(t, ts.URL+"/cmp/big.bin?partNumber=1&uploadId="+initResp.UploadID, creds, payload, "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	partBody, _ := io.ReadAll(part.Body)
	part.Body.Close()
	if part.StatusCode != http.StatusOK {
		t.Fatalf("part %d %s", part.StatusCode, partBody)
	}
	sum := md5.Sum(payload)
	want := `"` + hex.EncodeToString(sum[:]) + `"`
	if part.Header.Get("ETag") != want {
		t.Fatalf("part etag %s want %s", part.Header.Get("ETag"), want)
	}
	complete := `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>` + want + `</ETag></Part></CompleteMultipartUpload>`
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/cmp/big.bin?uploadId="+initResp.UploadID, strings.NewReader(complete))
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete %d %s", resp.StatusCode, raw)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/cmp/big.bin", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("got %q", body)
	}
}

func TestUnsignedChunkedTrailerChecksum(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "trl")

	payload := []byte("hello-aws")
	sum := crc32.ChecksumIEEE(payload)
	var buf [4]byte
	buf[0] = byte(sum >> 24)
	buf[1] = byte(sum >> 16)
	buf[2] = byte(sum >> 8)
	buf[3] = byte(sum)
	good := base64.StdEncoding.EncodeToString(buf[:])
	resp := doUnsignedTrailer(t, ts.URL+"/trl/nine.bin", creds, payload, good)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put %d %s", resp.StatusCode, raw)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/trl/nine.bin", nil)
	signRequest(t, req, creds, "us-east-1")
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	body, _ := io.ReadAll(got.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("got %q", body)
	}

	bad := doUnsignedTrailer(t, ts.URL+"/trl/bad.bin", creds, payload, base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 1}))
	raw, _ = io.ReadAll(bad.Body)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest || !bytes.Contains(raw, []byte("BadDigest")) {
		t.Fatalf("bad checksum %d %s", bad.StatusCode, raw)
	}
}

func TestChunkSignatureAndLengthRejected(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "bad")

	payload := []byte("hello-aws")
	req := signedChunkRequest(t, ts.URL+"/bad/sig.bin", creds, payload, "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	framed, _ := io.ReadAll(req.Body)
	for i := range framed {
		if framed[i] == '=' && i+2 < len(framed) {
			framed[i+1] ^= 1
			break
		}
	}
	req.Body = io.NopCloser(bytes.NewReader(framed))
	req.ContentLength = int64(len(framed))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !bytes.Contains(raw, []byte("SignatureDoesNotMatch")) {
		t.Fatalf("bad sig %d %s", resp.StatusCode, raw)
	}

	req = signedChunkRequest(t, ts.URL+"/bad/len.bin", creds, payload, "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	req.Header.Set("X-Amz-Decoded-Content-Length", strconv.Itoa(len(payload)+1))
	// The length header is signed. Rebuild so the signed value is the wrong length.
	req = signedChunkRequestWithLength(t, ts.URL+"/bad/len.bin", creds, payload, int64(len(payload)+1), "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !bytes.Contains(raw, []byte("IncompleteBody")) {
		t.Fatalf("bad length %d %s", resp.StatusCode, raw)
	}
}

func doChunked(t *testing.T, url string, creds auth.Credentials, payload []byte, mode string) *http.Response {
	t.Helper()
	req := signedChunkRequest(t, url, creds, payload, mode)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func signedChunkRequest(t *testing.T, url string, creds auth.Credentials, payload []byte, mode string) *http.Request {
	t.Helper()
	return signedChunkRequestWithLength(t, url, creds, payload, int64(len(payload)), mode)
}

func signedChunkRequestWithLength(t *testing.T, url string, creds auth.Credentials, payload []byte, decoded int64, mode string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	req.Header.Set("Content-Encoding", "aws-chunked")
	req.Header.Set("X-Amz-Decoded-Content-Length", strconv.FormatInt(decoded, 10))
	if err := auth.SignRequest(req, creds, "us-east-1", "s3", mode); err != nil {
		t.Fatal(err)
	}
	sig, date, scope, region, dateStamp := chunkSeed(t, req)
	framed := auth.EncodeSignedChunks(creds.SecretKey, date, scope, region, dateStamp, sig, payload)
	req.Body = io.NopCloser(bytes.NewReader(framed))
	req.ContentLength = int64(len(framed))
	return req
}

func doUnsignedTrailer(t *testing.T, url string, creds auth.Credentials, payload []byte, crcB64 string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString(strconv.FormatInt(int64(len(payload)), 16))
	buf.WriteString("\r\n")
	buf.Write(payload)
	buf.WriteString("\r\n0\r\nx-amz-checksum-crc32:")
	buf.WriteString(crcB64)
	buf.WriteString("\r\n\r\n")
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	req.Header.Set("Content-Encoding", "aws-chunked")
	req.Header.Set("X-Amz-Decoded-Content-Length", strconv.Itoa(len(payload)))
	req.ContentLength = int64(buf.Len())
	if err := auth.SignRequest(req, creds, "us-east-1", "s3", "STREAMING-UNSIGNED-PAYLOAD-TRAILER"); err != nil {
		t.Fatal(err)
	}
	// SignRequest does not read the body. Put it back because NewRequest already set it,
	// and SignRequest leaves the body in place.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func chunkSeed(t *testing.T, req *http.Request) (sig, date, scope, region, dateStamp string) {
	t.Helper()
	authz := req.Header.Get("Authorization")
	for _, part := range strings.Split(strings.TrimPrefix(authz, "AWS4-HMAC-SHA256 "), ", ") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "Credential":
			fields := strings.Split(v, "/")
			if len(fields) != 5 {
				t.Fatalf("credential %s", v)
			}
			dateStamp = fields[1]
			region = fields[2]
			scope = strings.Join(fields[1:], "/")
		case "Signature":
			sig = v
		}
	}
	date = req.Header.Get("X-Amz-Date")
	if sig == "" || date == "" || scope == "" {
		t.Fatalf("seed auth %s", authz)
	}
	return sig, date, scope, region, dateStamp
}
