package kafkasink

import (
	"errors"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
)

func TestExplainPublishErrorMissingTopic(t *testing.T) {
	err := explainPublishError(kerr.UnknownTopicOrPartition)
	if err == nil || !strings.Contains(err.Error(), "topic does not exist") {
		t.Fatalf("%v", err)
	}
	if !errors.Is(err, kerr.UnknownTopicOrPartition) && !strings.Contains(err.Error(), "UNKNOWN_TOPIC_OR_PARTITION") {
		t.Fatalf("broker code dropped: %v", err)
	}
	if got := explainPublishError(errors.New("dial tcp")); got == nil || got.Error() != "dial tcp" {
		t.Fatalf("other errors stay as-is: %v", got)
	}
	if explainPublishError(nil) != nil {
		t.Fatal("nil")
	}
}
