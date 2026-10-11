//go:build datasafe_sinkexec

package events

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/events/bridgeproto"
)

// KafkaBridgePath is the child process in the storage-server image.
// STORAGE_KAFKA_BRIDGE overrides it. The process is not started when brokers are empty.
const KafkaBridgePath = "/usr/lib/datasafe/datasafe-kafka-bridge"

// execSink talks to the franz-go child. The client is not in this process.
type execSink struct {
	path string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	waited chan struct{}
}

// NewKafkaSink starts the child. Call it only when brokers are set.
// An empty broker list returns an error and does not start a process.
func NewKafkaSink(cfg KafkaConfig) (EventSink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, fmt.Errorf("STORAGE_KAFKA_BROKERS is empty")
	}
	path := os.Getenv("STORAGE_KAFKA_BRIDGE")
	if path == "" {
		path = KafkaBridgePath
	}
	s := &execSink{path: path}
	if err := s.start(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *execSink) Name() string { return "kafka" }

func (s *execSink) Publish(ctx context.Context, e Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := NotificationJSON(e)
	if err != nil {
		return err
	}
	key := KafkaRecordKey(e.TenantID, e.Bucket, e.Key)
	msg, err := s.roundTrip(ctx, bridgeproto.KindPub, []byte(key), body)
	if err != nil {
		return err
	}
	if msg.Kind == bridgeproto.KindOK {
		return nil
	}
	text := "produce failed"
	if p, err := msg.Part(0); err == nil && len(p) > 0 {
		text = string(p)
	}
	return errors.New(text)
}

func (s *execSink) Health(ctx context.Context) (string, string) {
	msg, err := s.roundTrip(ctx, bridgeproto.KindHealth)
	if err != nil {
		return "degraded", err.Error()
	}
	if msg.Kind != bridgeproto.KindStatus {
		return "degraded", "unexpected health reply"
	}
	status, err1 := msg.Part(0)
	last, err2 := msg.Part(1)
	if err1 != nil || err2 != nil || len(status) == 0 {
		return "degraded", "short health reply"
	}
	return string(status), string(last)
}

func (s *execSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked()
}

func (s *execSink) roundTrip(ctx context.Context, kind byte, parts ...[]byte) (bridgeproto.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return bridgeproto.Message{}, ctx.Err()
	}
	if s.cmd == nil {
		if err := s.start(); err != nil {
			return bridgeproto.Message{}, err
		}
	}
	if err := bridgeproto.Write(s.stdin, kind, parts...); err != nil {
		_ = s.stopLocked()
		return bridgeproto.Message{}, err
	}
	type result struct {
		msg bridgeproto.Message
		err error
	}
	stdout := s.stdout
	ch := make(chan result, 1)
	go func() {
		msg, err := bridgeproto.Read(stdout)
		ch <- result{msg, err}
	}()
	select {
	case <-ctx.Done():
		_ = s.stopLocked()
		return bridgeproto.Message{}, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			_ = s.stopLocked()
			return bridgeproto.Message{}, res.err
		}
		return res.msg, nil
	}
}

func (s *execSink) start() error {
	if _, err := os.Stat(s.path); err != nil {
		return fmt.Errorf("STORAGE_KAFKA_BROKERS is set but the client program at %s is not available: %w", s.path, err)
	}
	cmd := exec.Command(s.path)
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdout)
	waited := make(chan struct{})
	s.waited = waited
	errBuf := &limitedBuf{max: 2048}
	go func() {
		_, _ = io.Copy(errBuf, stderr)
	}()
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()
	type result struct {
		msg bridgeproto.Message
		err error
	}
	rd := s.stdout
	ch := make(chan result, 1)
	go func() {
		msg, err := bridgeproto.Read(rd)
		ch <- result{msg, err}
	}()
	select {
	case <-time.After(15 * time.Second):
		_ = s.stopLocked()
		return fmt.Errorf("client program at %s did not become ready", s.path)
	case <-s.waited:
		_ = s.stopLocked()
		extra := strings.TrimSpace(errBuf.String())
		if extra != "" {
			return fmt.Errorf("client program at %s exited during startup: %s", s.path, extra)
		}
		return fmt.Errorf("client program at %s exited during startup", s.path)
	case res := <-ch:
		if res.err != nil {
			_ = s.stopLocked()
			return res.err
		}
		if res.msg.Kind == bridgeproto.KindFail {
			text := "startup failed"
			if p, err := res.msg.Part(0); err == nil && len(p) > 0 {
				text = string(p)
			}
			_ = s.stopLocked()
			return errors.New(text)
		}
		if res.msg.Kind != bridgeproto.KindReady {
			_ = s.stopLocked()
			return fmt.Errorf("client program sent frame %d during startup", res.msg.Kind)
		}
		return nil
	}
}

// limitedBuf keeps the start of a child process's stderr for a startup error.
type limitedBuf struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.b) < l.max {
		n := l.max - len(l.b)
		if n > len(p) {
			n = len(p)
		}
		l.b = append(l.b, p[:n]...)
	}
	return len(p), nil
}

func (l *limitedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.b)
}

func (s *execSink) stopLocked() error {
	if s.cmd == nil {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	done := s.waited
	cmd := s.cmd
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
	s.waited = nil
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}
