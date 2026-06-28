package api

import (
	"testing"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestValidateLoggingConfigElasticsearch(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	err := validateLoggingConfig(metadata.LoggingConfig{
		Elasticsearch: metadata.LogSinkEndpoint{
			Enabled:  true,
			Address:  "http://localhost:9200",
			Username: "elastic",
		},
	})
	if err == nil {
		t.Fatal("expected error when username set without password or token")
	}
	if err := validateLoggingConfig(metadata.LoggingConfig{
		Elasticsearch: metadata.LogSinkEndpoint{
			Enabled:  true,
			Address:  "http://localhost:9200",
			Username: "elastic",
			Password: "secret",
		},
	}); err != nil {
		t.Fatalf("basic auth config: %v", err)
	}
	if err := validateLoggingConfig(metadata.LoggingConfig{
		Elasticsearch: metadata.LogSinkEndpoint{
			Enabled: true,
			Address: "http://localhost:9200",
			Token:   "apikey",
		},
	}); err != nil {
		t.Fatalf("api key config: %v", err)
	}
}
