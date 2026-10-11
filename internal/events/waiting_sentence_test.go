//go:build !datasafe_sinkexec

package events

import (
	"testing"

	"github.com/DirektorBani/datasafe/internal/events/kafkasink"
)

func TestWaitingSentenceMatchesProducer(t *testing.T) {
	if kafkasink.WaitingForConnection != WaitingForConnection {
		t.Fatalf("producer %q dispatcher %q", kafkasink.WaitingForConnection, WaitingForConnection)
	}
}
