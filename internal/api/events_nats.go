package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	nats "github.com/nats-io/nats.go"
)

type natsEventSink struct {
	mu       sync.Mutex
	url      string
	subj     string
	nc       *nats.Conn
	js       nats.JetStreamContext
	jsKnown  bool
	jsOn     bool
	subjects map[string]bool
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
	n.js = nil
	n.jsKnown = false
	n.jsOn = false
	n.subjects = nil
	return nil
}

// Publish sends the event to NATS. An empty STORAGE_NATS_URL is a disabled sink:
// it returns nil and does not dial.
//
// When the connected server has JetStream enabled and a stream covers
// subject + "." + event type, the publish waits for a JetStream ack. An ack
// failure is returned and the dispatcher retries. Otherwise the publish is
// core NATS: success means the server accepted the message, not that a
// subscriber received it. The JetStream decision is cached until reconnect.
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
		if n.nc != nil {
			n.nc.Close()
		}
		nc, err := nats.Connect(n.url)
		if err != nil {
			return err
		}
		n.nc = nc
		n.js = nil
		n.jsKnown = false
		n.jsOn = false
		n.subjects = nil
	}
	body, err := json.Marshal(map[string]any{
		"event":   e.Type,
		"payload": e.Payload,
	})
	if err != nil {
		return err
	}
	subject := n.subj + "." + e.Type
	if n.useJetStream(subject) {
		_, err = n.js.Publish(subject, body, nats.Context(ctx))
		return err
	}
	return n.nc.Publish(subject, body)
}

func (n *natsEventSink) useJetStream(subject string) bool {
	if !n.jsKnown {
		n.jsKnown = true
		js, err := n.nc.JetStream(nats.MaxWait(2 * time.Second))
		if err != nil {
			return false
		}
		if _, err := js.AccountInfo(); err != nil {
			return false
		}
		n.js = js
		n.jsOn = true
		n.subjects = map[string]bool{}
	}
	if !n.jsOn || n.js == nil {
		return false
	}
	if v, ok := n.subjects[subject]; ok {
		return v
	}
	name, err := n.js.StreamNameBySubject(subject)
	ok := err == nil && name != ""
	n.subjects[subject] = ok
	return ok
}
