package kms

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
)

const (
	magic0 = 'D'
	magic1 = 'S'
	magic2 = 'S'
	magic3 = 'E'

	formatVersion1 = 1
	headerLen      = 16
	nonceLen       = 12
	tagLen         = 16
	// DefaultChunk is the plaintext size of one format-1 chunk.
	DefaultChunk = 64 * 1024
)

// EncryptReader returns a format-1 body. Plaintext is pulled as the caller reads.
func EncryptReader(dek []byte, plain io.Reader, chunk int) (io.Reader, error) {
	if chunk <= 0 {
		chunk = DefaultChunk
	}
	aead, err := aeadFor(dek)
	if err != nil {
		return nil, err
	}
	return &encReader{aead: aead, src: plain, chunk: chunk}, nil
}

type encReader struct {
	aead    cipher.AEAD
	src     io.Reader
	chunk   int
	header  []byte
	pending []byte
	done    bool
	started bool
}

func (r *encReader) aad() []byte {
	h := make([]byte, headerLen)
	h[0], h[1], h[2], h[3] = magic0, magic1, magic2, magic3
	h[4] = formatVersion1
	binary.BigEndian.PutUint32(h[8:12], uint32(r.chunk))
	return h
}

func (r *encReader) Read(p []byte) (int, error) {
	if !r.started {
		r.header = r.aad()
		r.started = true
	}
	if len(r.header) > 0 {
		n := copy(p, r.header)
		r.header = r.header[n:]
		return n, nil
	}
	if len(r.pending) == 0 && !r.done {
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	if len(r.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *encReader) fill() error {
	buf := make([]byte, r.chunk)
	n, err := io.ReadFull(r.src, buf)
	if err == io.EOF {
		r.done = true
		return nil
	}
	if err == io.ErrUnexpectedEOF {
		r.done = true
	} else if err != nil {
		return err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := r.aead.Seal(nil, nonce, buf[:n], r.aad())
	frame := make([]byte, 4+nonceLen+len(sealed))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(sealed)))
	copy(frame[4:4+nonceLen], nonce)
	copy(frame[4+nonceLen:], sealed)
	r.pending = frame
	return nil
}

// DecryptReader opens a format-1 body.
func DecryptReader(dek []byte, src io.Reader) (io.Reader, error) {
	aead, err := aeadFor(dek)
	if err != nil {
		return nil, err
	}
	header := make([]byte, headerLen)
	if _, err := io.ReadFull(src, header); err != nil {
		return nil, err
	}
	if header[0] != magic0 || header[1] != magic1 || header[2] != magic2 || header[3] != magic3 || header[4] != formatVersion1 {
		return nil, errors.New("unknown encryption header")
	}
	chunk := int(binary.BigEndian.Uint32(header[8:12]))
	if chunk <= 0 {
		return nil, errors.New("invalid chunk size")
	}
	return &decReader{aead: aead, src: src, aad: header, chunk: chunk}, nil
}

type decReader struct {
	aead  cipher.AEAD
	src   io.Reader
	aad   []byte
	chunk int
	plain []byte
	eof   bool
}

func (r *decReader) Read(p []byte) (int, error) {
	if len(r.plain) == 0 && !r.eof {
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	if len(r.plain) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.plain)
	r.plain = r.plain[n:]
	return n, nil
}

func (r *decReader) fill() error {
	var lenBuf [4]byte
	_, err := io.ReadFull(r.src, lenBuf[:])
	if err == io.EOF {
		r.eof = true
		return nil
	}
	if err != nil {
		return err
	}
	n := int(binary.BigEndian.Uint32(lenBuf[:]))
	if n < tagLen || n > r.chunk+tagLen {
		return errors.New("invalid encryption chunk")
	}
	buf := make([]byte, nonceLen+n)
	if _, err := io.ReadFull(r.src, buf); err != nil {
		return err
	}
	plain, err := r.aead.Open(nil, buf[:nonceLen], buf[nonceLen:], r.aad)
	if err != nil {
		return err
	}
	r.plain = plain
	return nil
}
