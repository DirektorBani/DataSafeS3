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
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/events/bridgeproto"
	"github.com/DirektorBani/datasafe/internal/events/kafkasink"
)

const (
	// healthTimeout bounds the broker ping behind a health frame.
	healthTimeout = 2 * time.Second
	// publishTimeout bounds one produce so a stopped broker returns an error
	// on stdout instead of leaving the parent blocked on the pipe.
	publishTimeout = 12 * time.Second
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
			} else if err := publish(prod, string(key), body); err != nil {
				_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte(oneLine(err.Error())))
			} else {
				_ = bridgeproto.Write(out, bridgeproto.KindOK)
			}
		case bridgeproto.KindHealth:
			status, last := probe(prod)
			_ = bridgeproto.Write(out, bridgeproto.KindStatus, []byte(status), []byte(oneLine(last)))
		default:
			_ = bridgeproto.Write(out, bridgeproto.KindErr, []byte(fmt.Sprintf("unknown frame %d", msg.Kind)))
		}
		if err := out.Flush(); err != nil {
			os.Exit(1)
		}
	}
}

func publish(prod *kafkasink.Producer, key string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	return prod.Publish(ctx, key, body)
}

func probe(prod *kafkasink.Producer) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()
	return prod.Probe(ctx)
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
