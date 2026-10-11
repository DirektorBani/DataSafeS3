package api

import (
	"os"
	"strings"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func kafkaBrokersConfigured() bool {
	return strings.TrimSpace(os.Getenv("STORAGE_KAFKA_BROKERS")) != ""
}

// syncKafkaSink registers the producer only when brokers are set.
// An empty STORAGE_KAFKA_BROKERS leaves no client and no connection.
// Invalid SASL/TLS config is returned to the caller so startup fails.
func (s *Server) syncKafkaSink() error {
	if s == nil || s.eventDisp == nil {
		return nil
	}
	if !kafkaBrokersConfigured() {
		s.eventDisp.Unregister("kafka")
		return nil
	}
	if s.eventDisp.HasSink("kafka") {
		return nil
	}
	cfg, err := events.KafkaConfigFromEnv()
	if err != nil {
		return err
	}
	sink, err := events.NewKafkaSink(cfg)
	if err != nil {
		return err
	}
	s.eventDisp.Register(sink)
	return nil
}

// noteKafkaBrokers writes Activity when the broker list differs from the
// value stored on the previous start. Equal values, including both empty,
// do not write a row and do not log.
func (s *Server) noteKafkaBrokers() {
	if s == nil || s.meta == nil {
		return
	}
	brokers := strings.TrimSpace(os.Getenv("STORAGE_KAFKA_BROKERS"))
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		return
	}
	if cfg.KafkaBrokers == brokers {
		return
	}
	prev := cfg.KafkaBrokers
	cfg.KafkaBrokers = brokers
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		return
	}
	if prev == "" && brokers == "" {
		return
	}
	s.logActivityAs("system", "", metadata.ActionKafkaBrokersChanged, "events", brokers)
}
