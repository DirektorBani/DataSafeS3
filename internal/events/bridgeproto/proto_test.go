package bridgeproto

import (
	"bytes"
	"io"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, KindPub, []byte("ten/b/obj"), []byte(`{"Records":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := Write(&buf, KindOK); err != nil {
		t.Fatal(err)
	}
	if err := Write(&buf, KindStatus, []byte("degraded"), []byte("down")); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindPub || string(got.Parts[0]) != "ten/b/obj" || string(got.Parts[1]) != `{"Records":[]}` {
		t.Fatalf("%#v", got)
	}
	got, err = Read(&buf)
	if err != nil || got.Kind != KindOK || len(got.Parts) != 0 {
		t.Fatalf("%v %#v", err, got)
	}
	got, err = Read(&buf)
	if err != nil || got.Kind != KindStatus || string(got.Parts[1]) != "down" {
		t.Fatalf("%v %#v", err, got)
	}
	if _, err := Read(&buf); err != io.EOF {
		t.Fatalf("eof %v", err)
	}
}

func TestRejectsHugePart(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{KindErr, 0, 1})
	var lenb [4]byte
	lenb[0] = 0x7f
	buf.Write(lenb[:])
	if _, err := Read(&buf); err == nil {
		t.Fatal("expected a size error")
	}
}
