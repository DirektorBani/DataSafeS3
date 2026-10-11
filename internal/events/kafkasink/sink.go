// Package kafkasink is the franz-go producer. The storage-server image does
// not import this package. The child process datasafe-kafka-bridge does, and
// the server starts that process only when STORAGE_KAFKA_BROKERS is set.
// go test and a normal go build import it through package events.
package kafkasink

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

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
type Producer struct {
	client *kgo.Client
	topic  string

	mu      sync.Mutex
	lastErr string
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
	return &Producer{client: cl, topic: cfg.Topic}, nil
}

// Publish sends body under key. key is tenant/bucket/key.
func (p *Producer) Publish(ctx context.Context, key string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rec := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(key),
		Value: body,
	}
	err := p.client.ProduceSync(ctx, rec).FirstErr()
	p.note(err)
	return err
}

func (p *Producer) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	p.client.Close()
	return nil
}

// Health pings the cluster when the last publish succeeded. "connected"
// means the last produce plus this ping succeeded. It does not mean a
// consumer group is caught up.
func (p *Producer) Health(ctx context.Context) (string, string) {
	p.mu.Lock()
	last := p.lastErr
	p.mu.Unlock()
	if last != "" {
		return "degraded", last
	}
	if err := p.client.Ping(ctx); err != nil {
		return "degraded", err.Error()
	}
	return "connected", ""
}

func (p *Producer) note(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.lastErr = ""
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
