// Command datasafe-kafka-bridge is the franz-go producer for the storage-server
// image. The server starts it only when STORAGE_KAFKA_BROKERS is set. The
// process reads bridgeproto frames on stdin and writes replies on stdout.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/events/bridgeproto"
	"github.com/DirektorBani/datasafe/internal/events/kafkasink"
)

func main() {
	out := bufio.NewWriter(os.Stdout)
	fail := func(err error) {
		_ = bridgeproto.Write(out, bridgeproto.KindFail, []byte(err.Error()))
		_ = out.Flush()
		os.Exit(1)
	}
	cfg, err := events.KafkaConfigFromEnv()
	if err != nil {
		fail(err)
	}
	prod, err := kafkasink.New(kafkasink.Config{
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
	})
	if err != nil {
		fail(err)
	}
	defer prod.Close()
	if err := bridgeproto.Write(out, bridgeproto.KindReady); err != nil {
		os.Exit(1)
	}
	if err := out.Flush(); err != nil {
		os.Exit(1)
	}
	in := bufio.NewReader(os.Stdin)
	for {
		msg, err := bridgeproto.Read(in)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			os.Exit(1)
		}
		switch msg.Kind {
		case bridgeproto.KindPub:
			key, err1 := msg.Part(0)
			body, err2 := msg.Part(1)
			if err1 != nil || err2 != nil {
				_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte("short publish frame"))
			} else if err := prod.Publish(context.Background(), string(key), body); err != nil {
				_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte(oneLine(err.Error())))
			} else {
				_ = bridgeproto.Write(out, bridgeproto.KindOK)
			}
		case bridgeproto.KindHealth:
			status, last := prod.Health(context.Background())
			_ = bridgeproto.Write(out, bridgeproto.KindStatus, []byte(status), []byte(oneLine(last)))
		default:
			_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte(fmt.Sprintf("unknown frame %d", msg.Kind)))
		}
		if err := out.Flush(); err != nil {
			os.Exit(1)
		}
	}
}

func oneLine(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			out = append(out, ' ')
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}
