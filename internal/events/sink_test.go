package events

import (
	"context"
	"errors"
	"testing"
)

type memSink struct {
	name   string
	events []NotificationEvent
	closed bool
}

func (m *memSink) Name() string { return m.name }

func (m *memSink) Publish(ctx context.Context, e NotificationEvent) error {
	if m.closed {
		return errors.New("closed")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.events = append(m.events, e)
	return nil
}

func (m *memSink) Close() error {
	m.closed = true
	return nil
}

func TestEventSinkContract(t *testing.T) {
	var s EventSink = &memSink{name: "mem"}
	if s.Name() != "mem" {
		t.Fatalf("Name=%q", s.Name())
	}
	e := NotificationEvent{Name: "s3:ObjectCreated:Put", Bucket: "b", Key: "k", Size: 3}
	if err := s.Publish(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(context.Background(), e); err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestEventSinkRespectsCanceledContext(t *testing.T) {
	s := &memSink{name: "mem"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Publish(ctx, NotificationEvent{Name: "x"}); err == nil {
		t.Fatal("expected ctx error")
	}
}
