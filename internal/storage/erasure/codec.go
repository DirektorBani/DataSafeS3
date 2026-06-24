// Package erasure provides a minimal 2+1 XOR parity codec for Community Edition HA data paths.
// Production hyperscale erasure is out of scope; this supports dev/lab shard layout validation.
package erasure

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidShardCount = errors.New("erasure: invalid shard count")
	ErrShardTooShort     = errors.New("erasure: shard shorter than expected")
	ErrMissingShard      = errors.New("erasure: missing data shard")
)

// Layout describes data+parity shard counts (e.g. 2+1 dev or 4+2 lab).
type Layout struct {
	DataShards   int
	ParityShards int
}

// DevLayout is the default 2 data + 1 parity XOR scheme.
func DevLayout() Layout { return Layout{DataShards: 2, ParityShards: 1} }

// LabLayout is a 4+2 style layout (4 data XOR parities).
func LabLayout() Layout { return Layout{DataShards: 4, ParityShards: 2} }

func (l Layout) total() int { return l.DataShards + l.ParityShards }

// Encode splits payload into data shards and computes XOR parity shard(s).
func Encode(data []byte, layout Layout) ([][]byte, error) {
	if layout.DataShards < 1 || layout.ParityShards < 1 {
		return nil, ErrInvalidShardCount
	}
	if len(data) == 0 {
		shards := make([][]byte, layout.total())
		for i := range shards {
			shards[i] = []byte{}
		}
		return shards, nil
	}
	chunk := (len(data) + layout.DataShards - 1) / layout.DataShards
	padded := make([]byte, chunk*layout.DataShards)
	copy(padded, data)
	shards := make([][]byte, layout.total())
	for i := 0; i < layout.DataShards; i++ {
		shards[i] = append([]byte(nil), padded[i*chunk:(i+1)*chunk]...)
	}
	for p := 0; p < layout.ParityShards; p++ {
		parity := make([]byte, chunk)
		for i := 0; i < layout.DataShards; i++ {
			if i%layout.ParityShards == p {
				for j := 0; j < chunk; j++ {
					parity[j] ^= shards[i][j]
				}
			}
		}
		shards[layout.DataShards+p] = parity
	}
	return shards, nil
}

// Decode reconstructs payload from data shards; if a data shard is nil, XOR parity recovers it.
func Decode(shards [][]byte, layout Layout, origLen int) ([]byte, error) {
	if len(shards) != layout.total() {
		return nil, fmt.Errorf("%w: got %d want %d", ErrInvalidShardCount, len(shards), layout.total())
	}
	if origLen < 0 {
		return nil, ErrShardTooShort
	}
	chunk := 0
	for _, s := range shards {
		if s != nil && len(s) > chunk {
			chunk = len(s)
		}
	}
	if chunk == 0 {
		return []byte{}, nil
	}
	// Recover single missing data shard using first parity.
	for i := 0; i < layout.DataShards; i++ {
		if shards[i] != nil {
			continue
		}
		parity := shards[layout.DataShards]
		if parity == nil {
			return nil, ErrMissingShard
		}
		recovered := make([]byte, chunk)
		copy(recovered, parity)
		for j := 0; j < layout.DataShards; j++ {
			if j == i || shards[j] == nil {
				continue
			}
			for k := 0; k < chunk; k++ {
				recovered[k] ^= shards[j][k]
			}
		}
		shards[i] = recovered
	}
	out := make([]byte, 0, chunk*layout.DataShards)
	for i := 0; i < layout.DataShards; i++ {
		if shards[i] == nil {
			return nil, ErrMissingShard
		}
		out = append(out, shards[i]...)
	}
	if origLen > len(out) {
		return nil, ErrShardTooShort
	}
	if origLen == 0 {
		return out, nil
	}
	return out[:origLen], nil
}
