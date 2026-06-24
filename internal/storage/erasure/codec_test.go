package erasure_test

import (
	"bytes"
	"testing"

	"github.com/DirektorBani/datasafe/internal/storage/erasure"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	data := []byte("datasafe erasure mvp payload for community ha")
	shards, err := erasure.Encode(data, erasure.DevLayout())
	if err != nil {
		t.Fatal(err)
	}
	got, err := erasure.Decode(shards, erasure.DevLayout(), len(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatalf("round trip mismatch")
	}
}

func TestRecoverSingleShardLoss(t *testing.T) {
	data := []byte("0123456789abcdef")
	shards, err := erasure.Encode(data, erasure.DevLayout())
	if err != nil {
		t.Fatal(err)
	}
	shards[0] = nil
	got, err := erasure.Decode(shards, erasure.DevLayout(), len(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatalf("recovery failed")
	}
}

func TestLabLayoutFourPlusTwo(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 251)
	}
	shards, err := erasure.Encode(data, erasure.LabLayout())
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 6 {
		t.Fatalf("want 6 shards got %d", len(shards))
	}
	got, err := erasure.Decode(shards, erasure.LabLayout(), len(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatal("lab layout round trip failed")
	}
}
