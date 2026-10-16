package kms

import (
	"crypto/aes"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
)

// ErrFormat1Header means the body is a format-1 envelope, not a 1.5.0 blob.
var ErrFormat1Header = errors.New("object envelope is missing encryption metadata")

// AuthenticateFormat0 reports whether r is a 1.5.0 AES-GCM blob sealed with
// this server key. It reads r in chunks and does not retain the body.
// A body that is too short or whose tag does not match is false, nil.
func (l *Local) AuthenticateFormat0(r io.Reader) (bool, error) {
	if l == nil {
		return false, ErrNotConfigured
	}
	kek, err := l.kek(0)
	if err != nil {
		return false, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return false, err
	}
	var nonce [12]byte
	if _, err := io.ReadFull(r, nonce[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return false, nil
		}
		return false, err
	}
	if nonce[0] == 'D' && nonce[1] == 'S' && nonce[2] == 'S' && nonce[3] == 'E' && nonce[4] == 1 {
		return false, ErrFormat1Header
	}
	var hBlock [16]byte
	block.Encrypt(hBlock[:], hBlock[:])
	g := newGhash(&hBlock)
	pending := make([]byte, 0, 64*1024+16)
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			if len(pending) > 16 {
				g.write(pending[:len(pending)-16])
				copy(pending, pending[len(pending)-16:])
				pending = pending[:16]
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}
	if len(pending) != 16 {
		return false, nil
	}
	var counter [16]byte
	copy(counter[:12], nonce[:])
	counter[15] = 1
	var tagMask [16]byte
	block.Encrypt(tagMask[:], counter[:])
	got := g.tag(&tagMask)
	if subtle.ConstantTimeCompare(got[:], pending) != 1 {
		return false, nil
	}
	return true, nil
}

// ghashState is GHASH over the ciphertext only (no additional data).
// The field layout matches NIST SP 800-38D as implemented by crypto/cipher.
type ghashState struct {
	y     gcmField
	table [16]gcmField
	block [16]byte
	n     int
	total int
}

type gcmField struct {
	low, high uint64
}

func newGhash(h *[16]byte) *ghashState {
	g := &ghashState{}
	x := gcmField{binary.BigEndian.Uint64(h[:8]), binary.BigEndian.Uint64(h[8:])}
	g.table[reverse4(1)] = x
	for i := 2; i < 16; i += 2 {
		g.table[reverse4(i)] = ghashDouble(g.table[reverse4(i/2)])
		g.table[reverse4(i+1)] = ghashAdd(g.table[reverse4(i)], x)
	}
	return g
}

func (g *ghashState) write(p []byte) {
	g.total += len(p)
	if g.n > 0 {
		need := 16 - g.n
		if len(p) < need {
			copy(g.block[g.n:], p)
			g.n += len(p)
			return
		}
		copy(g.block[g.n:], p[:need])
		g.absorbBlock(g.block[:])
		g.n = 0
		p = p[need:]
	}
	for len(p) >= 16 {
		g.absorbBlock(p[:16])
		p = p[16:]
	}
	if len(p) > 0 {
		copy(g.block[:], p)
		g.n = len(p)
	}
}

func (g *ghashState) absorbBlock(block []byte) {
	g.y.low ^= binary.BigEndian.Uint64(block[:8])
	g.y.high ^= binary.BigEndian.Uint64(block[8:])
	ghashMul(&g.table, &g.y)
}

func (g *ghashState) tag(mask *[16]byte) [16]byte {
	if g.n > 0 {
		var partial [16]byte
		copy(partial[:], g.block[:g.n])
		g.absorbBlock(partial[:])
		g.n = 0
	}
	var lenBlock [16]byte
	binary.BigEndian.PutUint64(lenBlock[8:], uint64(g.total)*8)
	g.absorbBlock(lenBlock[:])
	var out [16]byte
	binary.BigEndian.PutUint64(out[:8], g.y.low)
	binary.BigEndian.PutUint64(out[8:], g.y.high)
	for i := range out {
		out[i] ^= mask[i]
	}
	return out
}

func reverse4(i int) int {
	i = ((i << 2) & 0xc) | ((i >> 2) & 0x3)
	i = ((i << 1) & 0xa) | ((i >> 1) & 0x5)
	return i
}

func ghashAdd(x, y gcmField) gcmField {
	return gcmField{x.low ^ y.low, x.high ^ y.high}
}

func ghashDouble(x gcmField) gcmField {
	var d gcmField
	msb := x.high&1 == 1
	d.high = x.high >> 1
	d.high |= x.low << 63
	d.low = x.low >> 1
	if msb {
		d.low ^= 0xe100000000000000
	}
	return d
}

var ghashReduction = [16]uint64{
	0x0000, 0x1c20, 0x3840, 0x2460, 0x7080, 0x6ca0, 0x48c0, 0x54e0,
	0xe100, 0xfd20, 0xd940, 0xc560, 0x9180, 0x8da0, 0xa9c0, 0xb5e0,
}

func ghashMul(table *[16]gcmField, y *gcmField) {
	var z gcmField
	for i := 0; i < 2; i++ {
		word := y.high
		if i == 1 {
			word = y.low
		}
		for j := 0; j < 64; j += 4 {
			msw := z.high & 0xf
			z.high >>= 4
			z.high |= z.low << 60
			z.low >>= 4
			z.low ^= ghashReduction[msw] << 48
			t := table[word&0xf]
			z.low ^= t.low
			z.high ^= t.high
			word >>= 4
		}
	}
	*y = z
}
