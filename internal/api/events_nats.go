package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/DirektorBani/datasafe/internal/events"
	nats "github.com/nats-io/nats.go"
)

type natsEventSink struct {
	mu   sync.Mutex
	url  string
	subj string
	nc   *nats.Conn
}

func natsURLConfigured() bool {
	return strings.TrimSpace(os.Getenv("STORAGE_NATS_URL")) != ""
}

func newNATSEventSink() *natsEventSink {
	url := strings.TrimSpace(os.Getenv("STORAGE_NATS_URL"))
	subj := strings.TrimSpace(os.Getenv("STORAGE_NATS_SUBJECT"))
	if subj == "" {
		subj = "datasafe.events"
	}
	return &natsEventSink{url: url, subj: subj}
}

func (n *natsEventSink) Name() string { return "nats" }

func (n *natsEventSink) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.nc != nil {
		n.nc.Close()
		n.nc = nil
	}
	return nil
}

// Publish sends the event to NATS. An empty STORAGE_NATS_URL is a disabled sink:
// it returns nil and does not dial.
func (n *natsEventSink) Publish(ctx context.Context, e events.Event) error {
	if n.url == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.nc == nil || n.nc.Status() != nats.CONNECTED {
		nc, err := nats.Connect(n.url)
		if err != nil {
			return err
		}
		n.nc = nc
	}
	body, err := json.Marshal(map[string]any{
		"event":   e.Type,
		"payload": e.Payload,
	})
	if err != nil {
		return err
	}
	return n.nc.Publish(n.subj+"."+e.Type, body)
}
