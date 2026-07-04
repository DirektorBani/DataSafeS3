package erasure

import (
	"bytes"

	reedsolomon "github.com/klauspost/reedsolomon"
)

// RSCodec implements 4+2 Reed-Solomon erasure coding (production profile).
type RSCodec struct {
	data   int
	parity int
	enc    reedsolomon.Encoder
}

// ProductionLayout returns the 4 data + 2 parity Reed-Solomon layout.
func ProductionLayout() Layout { return Layout{DataShards: 4, ParityShards: 2} }

// NewRSCodec creates a Reed-Solomon encoder for the given layout.
func NewRSCodec(layout Layout) (*RSCodec, error) {
	enc, err := reedsolomon.New(layout.DataShards, layout.ParityShards)
	if err != nil {
		return nil, err
	}
	return &RSCodec{data: layout.DataShards, parity: layout.ParityShards, enc: enc}, nil
}

func (c *RSCodec) Layout() Layout { return Layout{DataShards: c.data, ParityShards: c.parity} }

func (c *RSCodec) Encode(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		shards := make([][]byte, c.data+c.parity)
		for i := range shards {
			shards[i] = []byte{}
		}
		return shards, nil
	}
	shards, err := c.enc.Split(data)
	if err != nil {
		return nil, err
	}
	if err := c.enc.Encode(shards); err != nil {
		return nil, err
	}
	return shards, nil
}

func (c *RSCodec) Decode(shards [][]byte, origLen int) ([]byte, error) {
	if len(shards) != c.data+c.parity {
		return nil, ErrInvalidShardCount
	}
	if err := c.enc.Reconstruct(shards); err != nil {
		return nil, err
	}
	ok, err := c.enc.Verify(shards)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrMissingShard
	}
	outSize := origLen
	if outSize <= 0 {
		outSize = len(shards[0]) * c.data
	}
	var buf bytes.Buffer
	if err := c.enc.Join(&buf, shards, outSize); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if origLen >= 0 && origLen <= len(out) {
		return out[:origLen], nil
	}
	return out, nil
}
