package erasure

// ShardCodec splits and joins object payloads into erasure shards.
type ShardCodec interface {
	Layout() Layout
	Encode(data []byte) ([][]byte, error)
	Decode(shards [][]byte, origLen int) ([]byte, error)
}

// NewCodec selects XOR (dev) or Reed-Solomon (production) codec.
func NewCodec(layoutName string) (ShardCodec, error) {
	switch layoutName {
	case "production", "prod", "4+2":
		return NewRSCodec(ProductionLayout())
	default:
		return xorCodec{layout: DevLayout()}, nil
	}
}

type xorCodec struct {
	layout Layout
}

func (x xorCodec) Layout() Layout { return x.layout }

func (x xorCodec) Encode(data []byte) ([][]byte, error) { return Encode(data, x.layout) }

func (x xorCodec) Decode(shards [][]byte, origLen int) ([]byte, error) {
	return Decode(shards, x.layout, origLen)
}
