package federation_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/federation"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestProxyListObjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "2" {
			http.Error(w, "bad query", 400)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<Contents><Key>peer-a.txt</Key><LastModified>2026-06-22T10:00:00Z</LastModified><ETag>"abc"</ETag><Size>3</Size></Contents>
</ListBucketResult>`))
	}))
	defer srv.Close()

	peer := metadata.FederationCluster{Name: "peer-a", Endpoint: srv.URL}
	objs, err := federation.ProxyListObjects(context.Background(), peer, "b", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Key != "peer-a.txt" {
		t.Fatalf("unexpected %+v", objs)
	}
}

func TestMergeFederatedObjects(t *testing.T) {
	local := []metadata.ObjectRecord{{Key: "local.txt", Size: 1}}
	fed := []federation.FederatedObject{{Key: "local.txt", Size: 9}, {Key: "remote.txt", Size: 2, SourcePeer: "p"}}
	merged := federation.MergeFederatedObjects(local, fed)
	if len(merged) != 2 || merged[1].Key != "remote.txt" {
		t.Fatalf("merge failed: %+v", merged)
	}
}

func TestListXMLParse(t *testing.T) {
	var parsed struct {
		XMLName  xml.Name `xml:"ListBucketResult"`
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
	}
	if err := xml.Unmarshal([]byte(`<ListBucketResult><Contents><Key>x</Key></Contents></ListBucketResult>`), &parsed); err != nil {
		t.Fatal(err)
	}
	_ = time.Now()
}
