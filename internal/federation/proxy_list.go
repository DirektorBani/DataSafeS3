package federation

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// FederatedObject is a lightweight list entry from a peer cluster.
type FederatedObject struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
	StorageClass string
	SourcePeer   string
}

type listObjectsV2XML struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int64  `xml:"Size"`
		StorageClass string `xml:"StorageClass"`
	} `xml:"Contents"`
}

// ProxyListObjects fetches object keys from a peer S3 ListObjectsV2 endpoint.
func ProxyListObjects(ctx context.Context, peer metadata.FederationCluster, bucket, prefix string, maxKeys int) ([]FederatedObject, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	base := strings.TrimSuffix(peer.Endpoint, "/")
	u, err := url.Parse(base + "/" + bucket)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("list-type", "2")
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	q.Set("max-keys", fmt.Sprintf("%d", maxKeys))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	applyFederationAuth(req)

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("peer list returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed listObjectsV2XML
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	out := make([]FederatedObject, 0, len(parsed.Contents))
	for _, c := range parsed.Contents {
		mod, _ := time.Parse(time.RFC3339, c.LastModified)
		out = append(out, FederatedObject{
			Key:          c.Key,
			Size:         c.Size,
			ETag:         strings.Trim(c.ETag, `"`),
			LastModified: mod,
			StorageClass: c.StorageClass,
			SourcePeer:   peer.Name,
		})
	}
	return out, nil
}

func applyFederationAuth(req *http.Request) {
	ak := strings.TrimSpace(os.Getenv("STORAGE_FEDERATION_ACCESS_KEY"))
	sk := strings.TrimSpace(os.Getenv("STORAGE_FEDERATION_SECRET_KEY"))
	if ak == "" {
		ak = strings.TrimSpace(os.Getenv("STORAGE_ACCESS_KEY"))
	}
	if sk == "" {
		sk = strings.TrimSpace(os.Getenv("STORAGE_SECRET_KEY"))
	}
	if ak != "" && sk != "" {
		req.SetBasicAuth(ak, sk)
	}
}

// MergeFederatedObjects merges peer listings into local records (local wins on key collision).
func MergeFederatedObjects(local []metadata.ObjectRecord, federated []FederatedObject) []metadata.ObjectRecord {
	if len(federated) == 0 {
		return local
	}
	seen := make(map[string]struct{}, len(local))
	for _, o := range local {
		seen[o.Key] = struct{}{}
	}
	for _, f := range federated {
		if _, ok := seen[f.Key]; ok {
			continue
		}
		local = append(local, metadata.ObjectRecord{
			Key:          f.Key,
			Size:         f.Size,
			ETag:         f.ETag,
			LastModified: f.LastModified,
			StorageClass: f.StorageClass,
			Metadata:     map[string]string{"x-datasafe-federation-peer": f.SourcePeer},
		})
		seen[f.Key] = struct{}{}
	}
	return local
}
