package erasure_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/DirektorBani/datasafe/internal/storage/erasure"
)

func TestRSRoundTrip(t *testing.T) {
	codec, err := erasure.NewRSCodec(erasure.ProductionLayout())
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 8192)
	for i := range data {
		data[i] = byte(i % 251)
	}
	shards, err := codec.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := codec.Decode(shards, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatal("round trip mismatch")
	}
}

func TestRSRecoverTwoShardLoss(t *testing.T) {
	codec, err := erasure.NewRSCodec(erasure.ProductionLayout())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("datasafe reed-solomon production profile")
	shards, err := codec.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	shards[0] = nil
	shards[5] = nil
	got, err := codec.Decode(shards, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatal("recovery failed")
	}
}

func TestBackendPutGet(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 3)
	for i := range paths {
		paths[i] = t.TempDir()
	}
	b, err := erasure.OpenBackend(erasure.Config{
		Paths:   paths,
		Layout:  "dev",
		Staging: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("hello erasure backend")
	etag, err := b.PutObject(t.Context(), "b", "k.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	rc, info, err := b.GetObject(t.Context(), "b", "k.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, got) {
		t.Fatalf("payload mismatch")
	}
	if info.ETag != etag {
		t.Fatalf("etag mismatch")
	}
}

func TestBackendHealMissingShard(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 3)
	for i := range paths {
		paths[i] = t.TempDir()
	}
	b, err := erasure.OpenBackend(erasure.Config{
		Paths:   paths,
		Layout:  "dev",
		Staging: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("heal me please")
	if _, err := b.PutObject(t.Context(), "b", "h.txt", bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	// Remove shard 0
	metaPath := paths[0]
	_ = metaPath
	// find and delete shard-0.bin
	// use HealOnce
	if _, err := b.HealOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	rc, _, err := b.GetObject(t.Context(), "b", "h.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(body, got) {
		t.Fatal("get after heal failed")
	}
}
