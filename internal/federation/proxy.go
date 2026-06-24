package federation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// ProxyGetObject fetches an object from a peer S3 endpoint (basic federation routing).
func ProxyGetObject(ctx context.Context, endpoint, bucket, key string) ([]byte, string, error) {
	url := strings.TrimSuffix(endpoint, "/") + "/" + bucket + "/" + key
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("peer returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	return data, ct, nil
}

// TestConnectivity probes peer health and optional object path.
func TestConnectivity(endpoint string) (status string, detail string) {
	url := strings.TrimSuffix(endpoint, "/") + "/healthz"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "offline", err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "degraded", fmt.Sprintf("healthz HTTP %d", resp.StatusCode)
	}
	return "healthy", "healthz ok"
}
