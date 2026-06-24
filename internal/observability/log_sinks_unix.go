//go:build !windows

package observability

import (
	"encoding/json"
	"log/slog"
	"log/syslog"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func buildPlatformLogSinks(cfg metadata.LoggingConfig) []LogSink {
	if !cfg.Syslog.Enabled {
		return nil
	}
	s, err := newSyslogSink(cfg.Syslog)
	if err != nil {
		slog.Warn("syslog sink disabled", "err", err, "address", cfg.Syslog.Address)
		return nil
	}
	return []LogSink{s}
}

type syslogSink struct {
	w *syslog.Writer
}

func newSyslogSink(cfg metadata.LogSinkEndpoint) (*syslogSink, error) {
	tag := "datasafe"
	network := "udp"
	addr := cfg.Address
	if len(addr) > 6 && addr[:6] == "tcp://" {
		network = "tcp"
		addr = addr[6:]
	} else if len(addr) > 6 && addr[:6] == "udp://" {
		addr = addr[6:]
	}
	w, err := syslog.Dial(network, addr, syslog.LOG_INFO|syslog.LOG_USER, tag)
	if err != nil {
		return nil, err
	}
	return &syslogSink{w: w}, nil
}

func (s *syslogSink) Name() string { return "syslog" }
func (s *syslogSink) Emit(record map[string]any) error {
	b, _ := json.Marshal(record)
	return s.w.Info(string(b))
}
func (s *syslogSink) Close() error { return s.w.Close() }
