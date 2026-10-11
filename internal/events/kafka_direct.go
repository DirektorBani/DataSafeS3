//go:build !datasafe_sinkexec

package events

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/events/kafkasink"
)

// kafkaProducer is the in-process adapter. The container image does not
// compile this file. It starts datasafe-kafka-bridge instead.
type kafkaProducer struct {
	p *kafkasink.Producer
}

// NewKafkaSink builds the producer in-process. The container image is built
// with the datasafe_sinkexec tag instead, so this file is not in that binary.
func NewKafkaSink(cfg KafkaConfig) (EventSink, error) {
	p, err := kafkasink.New(kafkaSinkConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &kafkaProducer{p: p}, nil
}

func (k *kafkaProducer) Name() string { return "kafka" }

func (k *kafkaProducer) Publish(ctx context.Context, e Event) error {
	body, err := NotificationJSON(e)
	if err != nil {
		return err
	}
	return k.p.Publish(ctx, KafkaRecordKey(e.TenantID, e.Bucket, e.Key), body)
}

func (k *kafkaProducer) Close() error { return k.p.Close() }

func (k *kafkaProducer) Health(ctx context.Context) (string, string) { return k.p.Health(ctx) }

func kafkaSinkConfig(cfg KafkaConfig) kafkasink.Config {
	return kafkasink.Config{
		Brokers:       cfg.Brokers,
		Topic:         cfg.Topic,
		ClientID:      cfg.ClientID,
		TLS:           cfg.TLS,
		CAFile:        cfg.CAFile,
		CertFile:      cfg.CertFile,
		KeyFile:       cfg.KeyFile,
		SASLMechanism: cfg.SASLMechanism,
		SASLUser:      cfg.SASLUser,
		SASLPassword:  cfg.SASLPassword,
	}
}
