package events

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

// KafkaSink publishes NotificationJSON with acks=all and the franz-go
// idempotent producer (the default when idempotent writes are not disabled).
// The record key is tenant/bucket/key.
type KafkaSink struct {
	client *kgo.Client
	topic  string

	mu      sync.Mutex
	lastErr string
}

// NewKafkaSink builds a producer. Call it only when cfg.Enabled() is true.
// A disabled config returns an error and does not dial.
func NewKafkaSink(cfg KafkaConfig) (*KafkaSink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, fmt.Errorf("STORAGE_KAFKA_BROKERS is empty")
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
	return &KafkaSink{client: cl, topic: cfg.Topic}, nil
}

func (k *KafkaSink) Name() string { return "kafka" }

func (k *KafkaSink) Publish(ctx context.Context, e Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := NotificationJSON(e)
	if err != nil {
		return err
	}
	rec := &kgo.Record{
		Topic: k.topic,
		Key:   []byte(KafkaRecordKey(e.TenantID, e.Bucket, e.Key)),
		Value: body,
	}
	err = k.client.ProduceSync(ctx, rec).FirstErr()
	k.note(err)
	return err
}

func (k *KafkaSink) Close() error {
	if k == nil || k.client == nil {
		return nil
	}
	k.client.Close()
	return nil
}

// Health pings the cluster when the last publish succeeded. "connected"
// means the sink is registered and the last produce plus this ping succeeded.
// It does not mean a consumer group is caught up.
func (k *KafkaSink) Health(ctx context.Context) (string, string) {
	k.mu.Lock()
	last := k.lastErr
	k.mu.Unlock()
	if last != "" {
		return "degraded", last
	}
	if err := k.client.Ping(ctx); err != nil {
		return "degraded", err.Error()
	}
	return "connected", ""
}

func (k *KafkaSink) note(err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err == nil {
		k.lastErr = ""
		return
	}
	k.lastErr = err.Error()
}

func kafkaTLSConfig(cfg KafkaConfig) (*tls.Config, error) {
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

func kafkaSASL(cfg KafkaConfig) (sasl.Mechanism, error) {
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
