// Package bridgeproto is the length-prefixed stream between the storage
// server and the Kafka child process. It does not import a Kafka client.
package bridgeproto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// KindReady is the child's first message after the producer is constructed.
	KindReady byte = 1
	// KindFail is a startup error. The child exits after sending it.
	KindFail byte = 2
	// KindPub asks the child to produce one record. Parts are key, body.
	KindPub byte = 3
	// KindHealth asks for connected or degraded. Parts of the reply are status, error.
	KindHealth byte = 4
	// KindOK is a successful produce.
	KindOK byte = 5
	// KindErr is a produce error. Parts are the error text.
	KindErr byte = 6
	// KindStatus is the health reply. Parts are status, error text.
	KindStatus byte = 7

	maxPart  = 8 << 20
	maxParts = 4
)

// Message is one frame.
type Message struct {
	Kind  byte
	Parts [][]byte
}

// Write encodes one message.
func Write(w io.Writer, kind byte, parts ...[]byte) error {
	if len(parts) > maxParts {
		return fmt.Errorf("bridge message has %d parts", len(parts))
	}
	var hdr [7]byte
	hdr[0] = kind
	binary.BigEndian.PutUint16(hdr[1:3], uint16(len(parts)))
	if _, err := w.Write(hdr[:3]); err != nil {
		return err
	}
	for _, p := range parts {
		if len(p) > maxPart {
			return fmt.Errorf("bridge part is %d bytes", len(p))
		}
		binary.BigEndian.PutUint32(hdr[3:7], uint32(len(p)))
		if _, err := w.Write(hdr[3:7]); err != nil {
			return err
		}
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// Read decodes one message. io.EOF means the peer closed the stream.
func Read(r io.Reader) (Message, error) {
	var hdr [3]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Message{}, err
	}
	n := int(binary.BigEndian.Uint16(hdr[1:3]))
	if n > maxParts {
		return Message{}, fmt.Errorf("bridge message has %d parts", n)
	}
	msg := Message{Kind: hdr[0], Parts: make([][]byte, n)}
	var lenb [4]byte
	for i := 0; i < n; i++ {
		if _, err := io.ReadFull(r, lenb[:]); err != nil {
			return Message{}, err
		}
		sz := binary.BigEndian.Uint32(lenb[:])
		if sz > maxPart {
			return Message{}, fmt.Errorf("bridge part is %d bytes", sz)
		}
		buf := make([]byte, sz)
		if _, err := io.ReadFull(r, buf); err != nil {
			return Message{}, err
		}
		msg.Parts[i] = buf
	}
	return msg, nil
}

// Part returns parts[i] or an error when the frame is short.
func (m Message) Part(i int) ([]byte, error) {
	if i < 0 || i >= len(m.Parts) {
		return nil, errors.New("short bridge message")
	}
	return m.Parts[i], nil
}
