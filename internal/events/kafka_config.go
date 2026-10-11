package events

import (
	"fmt"
	"os"
	"strings"
)

// KafkaConfig is the opt-in producer. Brokers empty means the sink is not
// constructed and nothing dials.
type KafkaConfig struct {
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

// Enabled is true when at least one broker address is set.
func (c KafkaConfig) Enabled() bool {
	return len(c.Brokers) > 0
}

// TLSOn is true when STORAGE_KAFKA_TLS is set or any TLS file is set.
func (c KafkaConfig) TLSOn() bool {
	return c.TLS || c.CAFile != "" || c.CertFile != "" || c.KeyFile != ""
}

// KafkaConfigFromEnv reads STORAGE_KAFKA_*. An empty STORAGE_KAFKA_BROKERS
// returns a disabled config and a nil error. A non-empty broker list with
// SASL PLAIN and no TLS is an error. *_FILE values override the matching
// variable and are trimmed of a trailing newline.
func KafkaConfigFromEnv() (KafkaConfig, error) {
	cfg := KafkaConfig{
		Brokers:       splitBrokers(os.Getenv("STORAGE_KAFKA_BROKERS")),
		Topic:         strings.TrimSpace(os.Getenv("STORAGE_KAFKA_TOPIC")),
		ClientID:      strings.TrimSpace(os.Getenv("STORAGE_KAFKA_CLIENT_ID")),
		TLS:           envBool("STORAGE_KAFKA_TLS"),
		CAFile:        strings.TrimSpace(os.Getenv("STORAGE_KAFKA_TLS_CA_FILE")),
		CertFile:      strings.TrimSpace(os.Getenv("STORAGE_KAFKA_TLS_CERT_FILE")),
		KeyFile:       strings.TrimSpace(os.Getenv("STORAGE_KAFKA_TLS_KEY_FILE")),
		SASLMechanism: strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_KAFKA_SASL_MECHANISM"))),
	}
	if cfg.Topic == "" {
		cfg.Topic = "datasafe.events"
	}
	if cfg.ClientID == "" {
		cfg.ClientID = "datasafe"
	}
	var err error
	if cfg.SASLUser, err = envOrFile("STORAGE_KAFKA_SASL_USERNAME"); err != nil {
		return KafkaConfig{}, err
	}
	if cfg.SASLPassword, err = envOrFile("STORAGE_KAFKA_SASL_PASSWORD"); err != nil {
		return KafkaConfig{}, err
	}
	if !cfg.Enabled() {
		return cfg, nil
	}
	if err := cfg.Validate(); err != nil {
		return KafkaConfig{}, err
	}
	return cfg, nil
}

// Validate checks a configured producer. A disabled config is valid.
func (c KafkaConfig) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if c.Topic == "" {
		return fmt.Errorf("STORAGE_KAFKA_TOPIC is empty")
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("STORAGE_KAFKA_TLS_CERT_FILE and STORAGE_KAFKA_TLS_KEY_FILE must be set together")
	}
	switch c.SASLMechanism {
	case "", "plain", "scram-sha-256", "scram-sha-512":
	default:
		return fmt.Errorf("STORAGE_KAFKA_SASL_MECHANISM must be PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512")
	}
	if c.SASLMechanism != "" {
		if c.SASLUser == "" || c.SASLPassword == "" {
			return fmt.Errorf("STORAGE_KAFKA_SASL_USERNAME and STORAGE_KAFKA_SASL_PASSWORD are required when SASL is set")
		}
	}
	if c.SASLMechanism == "plain" && !c.TLSOn() {
		return fmt.Errorf("STORAGE_KAFKA_SASL_MECHANISM=PLAIN requires TLS (set STORAGE_KAFKA_TLS=true or provide CA/cert/key files); refusing to start")
	}
	return nil
}

func splitBrokers(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envOrFile(key string) (string, error) {
	path := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if path == "" {
		return os.Getenv(key), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", key+"_FILE", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
