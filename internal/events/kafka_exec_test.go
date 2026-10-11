//go:build datasafe_sinkexec

package events

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
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
			if os.Getenv("CRASH_ON") == "publish" {
				_, _ = os.Stderr.WriteString("bridge crashed\n")
				os.Exit(2)
			}
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

func TestKafkaExecCrashRestartsWithBackoffAndKeepsRowsPending(t *testing.T) {
	var buf bytes.Buffer
	bridgeLogMu.Lock()
	prev := bridgeLogger
	bridgeLogger = slog.New(slog.NewTextHandler(&buf, nil))
	bridgeLogMu.Unlock()
	t.Cleanup(func() {
		bridgeLogMu.Lock()
		bridgeLogger = prev
		bridgeLogMu.Unlock()
	})

	t.Setenv("BE_FAKE_BRIDGE", "1")
	t.Setenv("CRASH_ON", "publish")
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
	if err := sink.Publish(ctx, ev); err == nil || !strings.Contains(err.Error(), "bridge exited") {
		t.Fatalf("crash publish %v", err)
	}
	os.Unsetenv("CRASH_ON")

	status, last := sink.(HealthReporter).Health(ctx)
	if status != "degraded" || !strings.Contains(last, "bridge exited") {
		t.Fatalf("while down %s %q", status, last)
	}
	start := time.Now()
	err = sink.Publish(ctx, ev)
	if err == nil {
		t.Fatal("publish while the bridge is down must fail so the row stays pending")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("publish while down took %s", time.Since(start))
	}

	now := time.Now().UTC()
	store := newMemOutbox()
	rec := NormalizeOutbox(OutboxRecord{
		ID:        "01CRASH0000000000000000000",
		EventType: "ObjectCreated",
		Bucket:    "b",
		Key:       "obj",
		TenantID:  "ten",
		Payload:   []byte(`{"size":1,"etag":"abc"}`),
		CreatedAt: now,
		Attempts:  map[string]SinkAttempt{"kafka": {Status: StatusPending, NextAttemptAt: now}},
	}, now)
	if _, err := store.InsertOutbox(ctx, rec); err != nil {
		t.Fatal(err)
	}
	// The child is still in backoff, so this delivery cannot ack the row.
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	d.lastPurge = now
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOutbox(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.Attempts["kafka"].Status != StatusPending {
		t.Fatalf("row should stay pending, got %+v", got)
	}

	deadline := time.Now().Add(3 * time.Second)
	var restarted bool
	for time.Now().Before(deadline) {
		if err := sink.Publish(ctx, ev); err == nil {
			restarted = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !restarted {
		t.Fatal("bridge was not restarted after backoff")
	}
	text := buf.String()
	if !strings.Contains(text, "event bridge exited") {
		t.Fatalf("restart was not logged: %s", text)
	}
	if !strings.Contains(text, "bridge crashed") {
		t.Fatalf("child stderr did not reach the server log: %s", text)
	}
	status, _ = sink.(HealthReporter).Health(ctx)
	if status != "connected" {
		t.Fatalf("after restart status %s", status)
	}
}

func TestKafkaExecBridgeHasNoTCPListener(t *testing.T) {
	t.Setenv("BE_FAKE_BRIDGE", "1")
	t.Setenv("STORAGE_KAFKA_BRIDGE", os.Args[0])
	t.Setenv("STORAGE_KAFKA_BROKERS", "127.0.0.1:1")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "")
	sink, err := NewKafkaSink(KafkaConfig{Brokers: []string{"127.0.0.1:1"}, Topic: "datasafe.events", ClientID: "datasafe"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	es := sink.(*execSink)
	es.mu.Lock()
	pid := 0
	if es.cmd != nil && es.cmd.Process != nil {
		pid = es.cmd.Process.Pid
	}
	es.mu.Unlock()
	if pid == 0 {
		t.Fatal("child pid")
	}
	inodes := socketInodes(t, pid)
	if listens(t, inodes) {
		t.Fatalf("child %d has a TCP listener", pid)
	}
}

func TestKafkaExecChildExitsWithParent(t *testing.T) {
	if os.Getenv("BE_BRIDGE_PARENT") == "1" {
		_ = os.Setenv("BE_FAKE_BRIDGE", "1")
		_ = os.Setenv("STORAGE_KAFKA_BRIDGE", os.Args[0])
		_ = os.Setenv("STORAGE_KAFKA_BROKERS", "127.0.0.1:1")
		_ = os.Unsetenv("STORAGE_KAFKA_SASL_MECHANISM")
		sink, err := NewKafkaSink(KafkaConfig{Brokers: []string{"127.0.0.1:1"}, Topic: "datasafe.events", ClientID: "datasafe"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		es := sink.(*execSink)
		es.mu.Lock()
		pid := es.cmd.Process.Pid
		es.mu.Unlock()
		fmt.Printf("%d\n", pid)
		os.Exit(0)
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BE_FAKE_BRIDGE=") || strings.HasPrefix(e, "BE_BRIDGE_PARENT=") || strings.HasPrefix(e, "CRASH_ON=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "BE_BRIDGE_PARENT=1")
	cmd := exec.Command(os.Args[0], "-test.run=^TestKafkaExecChildExitsWithParent$")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("parent helper: %v %s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	pidText := strings.TrimSpace(string(out))
	var pid int
	if _, err := fmt.Sscan(pidText, &pid); err != nil || pid <= 0 {
		t.Fatalf("pid %q: %v", pidText, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d still alive after the parent exited", pid)
}

func socketInodes(t *testing.T, pid int) map[string]bool {
	t.Helper()
	fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, fd := range fds {
		link, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
		if err != nil || !strings.HasPrefix(link, "socket:[") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
		out[inode] = true
	}
	return out
}

func listens(t *testing.T, inodes map[string]bool) bool {
	t.Helper()
	for _, name := range []string{"tcp", "tcp6"} {
		b, err := os.ReadFile("/proc/net/" + name)
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			if fields[3] == "0A" && inodes[fields[9]] {
				return true
			}
		}
	}
	return false
}

func processAlive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(b)
	i := strings.LastIndex(s, ")")
	if i < 0 || i+2 >= len(s) {
		return true
	}
	state := s[i+2]
	return state != 'Z' && state != 'X'
}
