// Package kafkasink is the franz-go producer. The storage-server image does
// not import this package. The child process datasafe-kafka-bridge does, and
// the server starts that process only when STORAGE_KAFKA_BROKERS is set.
// go test and a normal go build import it through package events.
package kafkasink

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

const (
	// dialTimeout and retryTimeout bound one broker RPC. Ping uses them so a
	// stopped broker cannot retry for minutes.
	dialTimeout  = 2 * time.Second
	retryTimeout = 2 * time.Second
	// deliveryTimeout bounds one produce when the caller set no deadline.
	// Record retries are otherwise unlimited.
	deliveryTimeout = 10 * time.Second
	probeTimeout    = 2 * time.Second
	probeInterval   = 5 * time.Second
)

// WaitingForConnection is the degraded reason before the first successful
// probe or publish. events.WaitingForConnection is the same sentence.
const WaitingForConnection = "waiting for first successful connection"

// Config is the producer. Brokers must be non-empty. TLS and SASL rules match
// events.KafkaConfig.Validate, which the caller runs first.
type Config struct {
	Brokers       []string
	Topic         string
	ClientID      string
	TLS           bool
	CAFile        string
	CertFile      string
	KeyFile       string
	SASLMechanism string
	SASLUser      string
	SASLPassword  string
}

// Producer publishes one record with acks=all and the franz-go idempotent
// producer (the default when idempotent writes are not disabled).
// Health reads a cache. A background probe and Publish update it. Neither
// the cache read nor a caller of Health dials the broker.
type Producer struct {
	client *kgo.Client
	topic  string

	mu            sync.Mutex
	lastErr       string
	publishFailed bool
	probed        bool
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// New builds a producer. An empty broker list returns an error and does not dial.
func New(cfg Config) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("STORAGE_KAFKA_BROKERS is empty")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("STORAGE_KAFKA_TOPIC is empty")
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ClientID(cfg.ClientID),
		kgo.DefaultProduceTopic(cfg.Topic),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		// Idempotent production is the client default. Do not call
		// DisableIdempotentWrite: retries of one produce keep a sequence.
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
		kgo.DialTimeout(dialTimeout),
		kgo.RetryTimeout(retryTimeout),
		kgo.RecordDeliveryTimeout(deliveryTimeout),
	}
	if cfg.tlsOn() {
		tlsCfg, err := kafkaTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.DialTLSConfig(tlsCfg))
	}
	if cfg.SASLMechanism != "" {
		mech, err := kafkaSASL(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mech))
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Producer{client: cl, topic: cfg.Topic, cancel: cancel}
	p.wg.Add(1)
	go p.probeLoop(ctx)
	return p, nil
}

// Publish sends body under key. key is tenant/bucket/key.
// A caller without a deadline is capped at deliveryTimeout so a stopped
// broker fails the attempt instead of blocking the dispatcher.
func (p *Producer) Publish(ctx context.Context, key string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, deliveryTimeout)
		defer cancel()
	}
	rec := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(key),
		Value: body,
	}
	err := explainPublishError(p.client.ProduceSync(ctx, rec).FirstErr())
	p.note(err)
	return err
}

// explainPublishError turns a missing topic into a stable sentence. The
// producer does not create topics. The broker error is kept after the sentence
// so logs still show UNKNOWN_TOPIC_OR_PARTITION. Delivery retries either way.
func explainPublishError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, kerr.UnknownTopicOrPartition) || strings.Contains(err.Error(), "UNKNOWN_TOPIC_OR_PARTITION") {
		return fmt.Errorf("topic does not exist: %w", err)
	}
	return err
}

func (p *Producer) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.client.Close()
	return nil
}

// Health returns the cached status. It does not ping and it does not wait
// on an in-flight produce. "connected" means a probe or a publish succeeded
// and the last publish did not fail. It does not mean a consumer group is
// caught up. Before the first probe finishes the status is degraded and the
// reason is WaitingForConnection.
func (p *Producer) Health(context.Context) (string, string) {
	return p.cached()
}

// Probe pings once, bounded by ctx, and updates the cache. Publish errors
// are kept when the ping succeeds: a live broker with a missing topic stays
// degraded. A ping failure does not hide a newer publish error.
func (p *Producer) Probe(ctx context.Context) (string, string) {
	if err := ctx.Err(); err != nil {
		p.notePing(err)
		return p.cached()
	}
	p.notePing(p.client.Ping(ctx))
	return p.cached()
}

func (p *Producer) probeLoop(ctx context.Context) {
	defer p.wg.Done()
	p.probe(ctx)
	t := time.NewTicker(probeInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.probe(ctx)
		}
	}
}

func (p *Producer) probe(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	p.notePing(p.client.Ping(pctx))
}

func (p *Producer) cached() (string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastErr != "" {
		return "degraded", p.lastErr
	}
	if !p.probed {
		return "degraded", WaitingForConnection
	}
	return "connected", ""
}

func (p *Producer) note(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probed = true
	if err == nil {
		p.lastErr = ""
		p.publishFailed = false
		return
	}
	p.lastErr = err.Error()
	p.publishFailed = true
}

func (p *Producer) notePing(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probed = true
	if err == nil {
		if !p.publishFailed {
			p.lastErr = ""
		}
		return
	}
	if p.publishFailed {
		return
	}
	p.lastErr = err.Error()
}

func (c Config) tlsOn() bool {
	return c.TLS || c.CAFile != "" || c.CertFile != "" || c.KeyFile != ""
}

func kafkaTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("STORAGE_KAFKA_TLS_CA_FILE: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("STORAGE_KAFKA_TLS_CA_FILE: no certificates")
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("STORAGE_KAFKA_TLS_CERT_FILE: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	return tlsCfg, nil
}

func kafkaSASL(cfg Config) (sasl.Mechanism, error) {
	switch cfg.SASLMechanism {
	case "plain":
		return plain.Auth{User: cfg.SASLUser, Pass: cfg.SASLPassword}.AsMechanism(), nil
	case "scram-sha-256":
		return scram.Auth{User: cfg.SASLUser, Pass: cfg.SASLPassword}.AsSha256Mechanism(), nil
	case "scram-sha-512":
		return scram.Auth{User: cfg.SASLUser, Pass: cfg.SASLPassword}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("STORAGE_KAFKA_SASL_MECHANISM must be PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512")
	}
}
