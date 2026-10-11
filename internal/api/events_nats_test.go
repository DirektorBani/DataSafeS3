package api

import (
	"context"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
)

func startNATS(t *testing.T, jetstream bool) *server.Server {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
		JetStream: jetstream,
	}
	if jetstream {
		opts.StoreDir = t.TempDir()
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats-server not ready")
	}
	return ns
}

func TestNATSJetStreamAck(t *testing.T) {
	ns := startNATS(t, true)
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "EVENTS",
		Subjects: []string{"datasafe.events.>"},
	}); err != nil {
		t.Fatal(err)
	}
	sink := &natsEventSink{url: ns.ClientURL(), subj: "datasafe.events"}
	t.Cleanup(func() { _ = sink.Close() })
	if err := sink.Publish(context.Background(), events.Event{
		Type:    "ObjectCreated",
		Payload: map[string]any{"bucket": "b", "key": "k"},
	}); err != nil {
		t.Fatal(err)
	}
	info, err := js.StreamInfo("EVENTS")
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("stream msgs %d", info.State.Msgs)
	}
	sub, err := js.SubscribeSync("datasafe.events.ObjectCreated", nats.Durable("e03"))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Ack(); err != nil {
		t.Fatal(err)
	}
	if len(msg.Data) == 0 || msg.Subject != "datasafe.events.ObjectCreated" {
		t.Fatalf("msg %s %s", msg.Subject, msg.Data)
	}
}

func TestNATSCorePublishWithoutJetStream(t *testing.T) {
	ns := startNATS(t, false)
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	sub, err := nc.SubscribeSync("datasafe.events.ObjectCreated")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	sink := &natsEventSink{url: ns.ClientURL(), subj: "datasafe.events"}
	t.Cleanup(func() { _ = sink.Close() })
	start := time.Now()
	if err := sink.Publish(context.Background(), events.Event{Type: "ObjectCreated", Payload: map[string]any{"key": "k"}}); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("core publish took %s", time.Since(start))
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.Data) == 0 {
		t.Fatal("empty core message")
	}
}
