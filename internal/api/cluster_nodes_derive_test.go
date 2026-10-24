package api

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestDeriveClusterNodesSingleNodeIsNotLocalhost9000(t *testing.T) {
	t.Setenv("STORAGE_ADDR", ":9001")
	got := deriveClusterNodes("node1", localProbeAddr(), nil)
	if len(got) != 1 || got[0].Address != "127.0.0.1:9001" || got[0].ID != "node1" || got[0].Role != "primary" {
		t.Fatalf("single node must probe its own port, got %+v", got)
	}
	t.Setenv("STORAGE_ADDR", "")
	if a := localProbeAddr(); a != "127.0.0.1:9000" {
		t.Fatalf("default port, got %s", a)
	}
}

func TestDeriveClusterNodesHAListsEveryRegisteredNode(t *testing.T) {
	now := time.Now()
	beats := []metadata.NodeHeartbeat{
		{NodeID: "node2", Address: "http://node2:9000", SeenAt: now},
		{NodeID: "node1", Address: "http://node1:9000", SeenAt: now},
		{NodeID: "node3", SeenAt: now}, // never registered an address: not probeable
	}
	got := deriveClusterNodes("node1", "127.0.0.1:9000", beats)
	if len(got) != 2 {
		t.Fatalf("want self + node2 (\"Nodes configured\" 2), got %+v", got)
	}
	if got[0].ID != "node1" || got[0].Address != "127.0.0.1:9000" {
		t.Fatalf("self first and probed on loopback, got %+v", got[0])
	}
	if got[1].ID != "node2" || got[1].Address != "http://node2:9000" {
		t.Fatalf("peer uses its registered address, got %+v", got[1])
	}
	for _, n := range got {
		if n.Address == "localhost:9000" {
			t.Fatalf("hardcoded localhost:9000 must not appear: %+v", got)
		}
	}
}
