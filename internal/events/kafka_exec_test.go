//go:build datasafe_sinkexec

package events

import (
	"bufio"
	"context"
	"os"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events/bridgeproto"
)

func TestMain(m *testing.M) {
	if os.Getenv("BE_FAKE_BRIDGE") == "1" {
		fakeBridge()
		return
	}
	os.Exit(m.Run())
}

func fakeBridge() {
	out := bufio.NewWriter(os.Stdout)
	_ = bridgeproto.Write(out, bridgeproto.KindReady)
	_ = out.Flush()
	in := bufio.NewReader(os.Stdin)
	failFirst := os.Getenv("FAIL_FIRST") == "1"
	var n int
	for {
		msg, err := bridgeproto.Read(in)
		if err != nil {
			return
		}
		switch msg.Kind {
		case bridgeproto.KindPub:
			n++
			if failFirst && n == 1 {
				_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte("first produce failed"))
			} else {
				_ = bridgeproto.Write(out, bridgeproto.KindOK)
			}
		case bridgeproto.KindHealth:
			_ = bridgeproto.Write(out, bridgeproto.KindStatus, []byte("connected"), []byte(""))
		default:
			_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte("unknown"))
		}
		_ = out.Flush()
	}
}

func TestKafkaExecFakeBridgeOrdersAndRestarts(t *testing.T) {
	t.Setenv("BE_FAKE_BRIDGE", "1")
	t.Setenv("FAIL_FIRST", "1")
	t.Setenv("STORAGE_KAFKA_BRIDGE", os.Args[0])
	t.Setenv("STORAGE_KAFKA_BROKERS", "127.0.0.1:1")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "")
	sink, err := NewKafkaSink(KafkaConfig{Brokers: []string{"127.0.0.1:1"}, Topic: "datasafe.events", ClientID: "datasafe"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	ctx := context.Background()
	ev := Event{TenantID: "ten", Bucket: "b", Key: "obj", Type: "ObjectCreated", CreatedAt: time.Now().UTC()}
	if err := sink.Publish(ctx, ev); err == nil || err.Error() != "first produce failed" {
		t.Fatalf("first publish %v", err)
	}
	ev.Type = "ObjectDeleted"
	if err := sink.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	status, last := sink.(HealthReporter).Health(ctx)
	if status != "connected" || last != "" {
		t.Fatalf("health %s %q", status, last)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("FAIL_FIRST")
	ev.Key = "other"
	ev.Type = "ObjectCreated"
	if err := sink.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
}
