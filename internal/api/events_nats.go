package api

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	nats "github.com/nats-io/nats.go"
)

type natsEventSink struct {
	mu   sync.Mutex
	url  string
	subj string
	nc   *nats.Conn
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

func (n *natsEventSink) Publish(event string, payload map[string]any) error {
	if n.url == "" {
		return nil
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
		"event":   event,
		"payload": payload,
	})
	if err != nil {
		return err
	}
	return n.nc.Publish(n.subj+"."+event, body)
}
