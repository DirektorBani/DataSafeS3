package kafkasink

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHealthDoesNotBlockOnUnreachableBroker(t *testing.T) {
	p, err := New(Config{
		Brokers:  []string{"192.0.2.1:9092"},
		Topic:    "datasafe.events",
		ClientID: "datasafe",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })

	deadline := time.Now().Add(8 * time.Second)
	var status, last string
	for {
		start := time.Now()
		status, last = p.Health(context.Background())
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("Health blocked for %s", elapsed)
		}
		if status != "degraded" {
			t.Fatalf("status %s", status)
		}
		if last == "" {
			t.Fatal("degraded status has an empty reason")
		}
		if last == WaitingForConnection {
			if time.Now().After(deadline) {
				t.Fatal("probe did not record a broker error")
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}
		break
	}
	if !strings.Contains(last, "timeout") && !strings.Contains(last, "refused") && !strings.Contains(last, "dial") && !strings.Contains(last, "deadline") {
		t.Fatalf("last_error %q", last)
	}
}
