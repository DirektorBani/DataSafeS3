package auth

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
	"hash/crc32"
	"hash/crc64"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	streamingSigned          = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"
	streamingSignedTrailer   = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD-TRAILER"
	streamingUnsignedTrailer = "STREAMING-UNSIGNED-PAYLOAD-TRAILER"
	maxChunkSize             = 128 << 20
)

var (
	ErrChunkSignature = errors.New("chunk signature mismatch")
	ErrMalformedChunk = errors.New("malformed aws-chunked body")
	ErrBadDigest      = errors.New("trailing checksum mismatch")
	ErrDecodedLength  = errors.New("decoded content length mismatch")
)

// UnwrapAWSChunked returns the decoded object bytes for an aws-chunked body.
// A request that is not aws-chunked is returned unchanged.
// Chunk signatures are checked for STREAMING-AWS4-HMAC-SHA256-PAYLOAD.
// Trailing checksums are checked when the body carries them.
func UnwrapAWSChunked(r *http.Request, secret string) (io.Reader, int64, error) {
	mode := streamingMode(r)
	if mode == "" {
		return r.Body, r.ContentLength, nil
	}
	decoded := int64(-1)
	if raw := strings.TrimSpace(r.Header.Get("X-Amz-Decoded-Content-Length")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return nil, 0, ErrMalformedChunk
		}
		decoded = n
	}
	signed := mode == streamingSigned || mode == streamingSignedTrailer
	trailers := mode == streamingSignedTrailer || mode == streamingUnsignedTrailer || r.Header.Get("X-Amz-Trailer") != ""
	cr := &chunkReader{
		in:       bufio.NewReader(r.Body),
		signed:   signed,
		trailers: trailers,
		want:     decoded,
		crc32:    crc32.NewIEEE(),
		crc32c:   crc32.New(crc32.MakeTable(crc32.Castagnoli)),
		sha1:     sha1.New(),
		sha256:   sha256.New(),
		crc64:    crc64.New(crc64NVMETable),
	}
	if signed {
		sig, date, scope, region, dateStamp, err := seedFromRequest(r)
		if err != nil {
			return nil, 0, err
		}
		cr.secret = secret
		cr.amzDate = date
		cr.scope = scope
		cr.region = region
		cr.dateStamp = dateStamp
		cr.prevSig = sig
	}
	return cr, decoded, nil
}

// EncodeSignedChunks frames payload as one signed data chunk and a zero chunk.
// seed is the Authorization signature. The result is what mc sends for
// STREAMING-AWS4-HMAC-SHA256-PAYLOAD.
func EncodeSignedChunks(secret, amzDate, scope, region, dateStamp, seed string, payload []byte) []byte {
	c := &chunkReader{
		secret: secret, amzDate: amzDate, scope: scope, region: region,
		dateStamp: dateStamp, prevSig: seed, signed: true,
	}
	var buf []byte
	write := func(data []byte) {
		sum := sha256.Sum256(data)
		sig := c.chunkSig(sum[:])
		c.prevSig = sig
		buf = append(buf, []byte(strconv.FormatInt(int64(len(data)), 16)+";chunk-signature="+sig+"\r\n")...)
		buf = append(buf, data...)
		if len(data) > 0 {
			buf = append(buf, '\r', '\n')
		}
	}
	write(payload)
	write(nil)
	return buf
}

func streamingMode(r *http.Request) string {
	switch r.Header.Get("X-Amz-Content-Sha256") {
	case streamingSigned, streamingSignedTrailer, streamingUnsignedTrailer:
		return r.Header.Get("X-Amz-Content-Sha256")
	}
	enc := strings.ToLower(r.Header.Get("Content-Encoding"))
	if strings.Contains(enc, "aws-chunked") {
		return streamingUnsignedTrailer
	}
	return ""
}

func seedFromRequest(r *http.Request) (sig, date, scope, region, dateStamp string, err error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		return "", "", "", "", "", ErrMalformedChunk
	}
	parts := parseAuthHeader(auth[len("AWS4-HMAC-SHA256 "):])
	sig = parts["Signature"]
	cred := parts["Credential"]
	fields := strings.Split(cred, "/")
	if sig == "" || len(fields) != 5 {
		return "", "", "", "", "", ErrMalformedChunk
	}
	date = r.Header.Get("X-Amz-Date")
	if date == "" {
		return "", "", "", "", "", ErrMalformedChunk
	}
	dateStamp = fields[1]
	region = fields[2]
	scope = strings.Join(fields[1:], "/")
	return sig, date, scope, region, dateStamp, nil
}

// crc64NVMETable is the reflected AWS CRC64NVME polynomial.
// crc64.MakeTable runs the reflected algorithm. The normal polynomial
// 0xAD93D23594C935A9 does not verify the aws-cli trailer.
// Check value of "123456789" is ae8b14860a799888.
var crc64NVMETable = crc64.MakeTable(0x9A6C9329AC4BC9B5)

type chunkReader struct {
	in        *bufio.Reader
	signed    bool
	trailers  bool
	secret    string
	amzDate   string
	scope     string
	region    string
	dateStamp string
	prevSig   string

	want int64
	got  int64

	left []byte
	err  error

	crc32  hash.Hash
	crc32c hash.Hash
	sha1   hash.Hash
	sha256 hash.Hash
	crc64  hash.Hash
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.left) > 0 {
		n := copy(p, c.left)
		c.left = c.left[n:]
		return n, nil
	}
	if c.err != nil {
		return 0, c.err
	}
	size, sig, err := c.readHeader()
	if err != nil {
		c.err = err
		return 0, err
	}
	if size > maxChunkSize {
		c.err = ErrMalformedChunk
		return 0, c.err
	}
	buf := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(c.in, buf); err != nil {
			c.err = ErrMalformedChunk
			return 0, c.err
		}
		if err := c.expectCRLF(); err != nil {
			c.err = err
			return 0, err
		}
	}
	if c.signed {
		sum := sha256.Sum256(buf)
		next := c.chunkSig(sum[:])
		if !strings.EqualFold(next, sig) {
			c.err = ErrChunkSignature
			return 0, c.err
		}
		c.prevSig = next
	}
	if size == 0 {
		if c.trailers {
			if err := c.readTrailers(); err != nil {
				c.err = err
				return 0, err
			}
		}
		if c.want >= 0 && c.got != c.want {
			c.err = ErrDecodedLength
			return 0, c.err
		}
		c.err = io.EOF
		return 0, io.EOF
	}
	_, _ = c.crc32.Write(buf)
	_, _ = c.crc32c.Write(buf)
	_, _ = c.sha1.Write(buf)
	_, _ = c.sha256.Write(buf)
	_, _ = c.crc64.Write(buf)
	c.got += int64(size)
	n := copy(p, buf)
	c.left = buf[n:]
	return n, nil
}

func (c *chunkReader) chunkSig(sum []byte) string {
	empty := sha256.Sum256(nil)
	sts := strings.Join([]string{
		"AWS4-HMAC-SHA256-PAYLOAD",
		c.amzDate,
		c.scope,
		c.prevSig,
		hex.EncodeToString(empty[:]),
		hex.EncodeToString(sum),
	}, "\n")
	return sign(c.secret, c.dateStamp, c.region, "s3", sts)
}

func (c *chunkReader) trailerSig(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	sts := strings.Join([]string{
		"AWS4-HMAC-SHA256-TRAILER",
		c.amzDate,
		c.scope,
		c.prevSig,
		hex.EncodeToString(sum[:]),
	}, "\n")
	return sign(c.secret, c.dateStamp, c.region, "s3", sts)
}

func (c *chunkReader) readHeader() (int, string, error) {
	line, err := c.readLine()
	if err != nil {
		return 0, "", ErrMalformedChunk
	}
	sig := ""
	hexSize := line
	if i := strings.Index(line, ";"); i >= 0 {
		hexSize = line[:i]
		rest := line[i+1:]
		const pfx = "chunk-signature="
		j := strings.Index(rest, pfx)
		if j < 0 {
			if c.signed {
				return 0, "", ErrMalformedChunk
			}
		} else {
			sig = rest[j+len(pfx):]
			if k := strings.IndexByte(sig, ';'); k >= 0 {
				sig = sig[:k]
			}
			sig = strings.TrimSpace(sig)
		}
	} else if c.signed {
		return 0, "", ErrMalformedChunk
	}
	if hexSize == "" {
		return 0, "", ErrMalformedChunk
	}
	n, err := strconv.ParseInt(hexSize, 16, 64)
	if err != nil || n < 0 {
		return 0, "", ErrMalformedChunk
	}
	return int(n), sig, nil
}

func (c *chunkReader) readTrailers() error {
	var lines []string
	var trailerSig string
	for {
		line, err := c.readLine()
		if err != nil {
			return ErrMalformedChunk
		}
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "x-amz-trailer-signature:") {
			trailerSig = strings.TrimSpace(line[len("x-amz-trailer-signature:"):])
			if err := c.finishTrailerBlock(); err != nil {
				return err
			}
			break
		}
		lines = append(lines, line)
	}
	if c.signed && c.trailers {
		canonical := ""
		if len(lines) > 0 {
			canonical = strings.Join(lines, "\n") + "\n"
		}
		if trailerSig == "" || !strings.EqualFold(c.trailerSig(canonical), trailerSig) {
			return ErrChunkSignature
		}
	}
	for _, line := range lines {
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		if !c.checksumOK(name, val) {
			return ErrBadDigest
		}
	}
	return nil
}

func (c *chunkReader) finishTrailerBlock() error {
	// Signed trailers end with CRLF after the signature line, then a blank line.
	// finishTrailerBlock consumes a following empty line when it is present.
	b, err := c.in.ReadByte()
	if err != nil {
		return nil
	}
	if b != '\r' {
		_ = c.in.UnreadByte()
		return nil
	}
	n, err := c.in.ReadByte()
	if err != nil || n != '\n' {
		return ErrMalformedChunk
	}
	return nil
}

func (c *chunkReader) checksumOK(name, b64 string) bool {
	var sum []byte
	switch name {
	case "x-amz-checksum-crc32":
		sum = c.crc32.Sum(nil)
	case "x-amz-checksum-crc32c":
		sum = c.crc32c.Sum(nil)
	case "x-amz-checksum-sha1":
		sum = c.sha1.Sum(nil)
	case "x-amz-checksum-sha256":
		sum = c.sha256.Sum(nil)
	case "x-amz-checksum-crc64nvme":
		sum = c.crc64.Sum(nil)
	default:
		return true
	}
	got, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return false
	}
	if len(got) != len(sum) {
		return false
	}
	for i := range got {
		if got[i] != sum[i] {
			return false
		}
	}
	return true
}

func (c *chunkReader) readLine() (string, error) {
	line, err := c.in.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	return line, nil
}

func (c *chunkReader) expectCRLF() error {
	b := make([]byte, 2)
	if _, err := io.ReadFull(c.in, b); err != nil || b[0] != '\r' || b[1] != '\n' {
		return ErrMalformedChunk
	}
	return nil
}
