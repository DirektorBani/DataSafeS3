package ha

import (
	"os"
	"strings"
	"testing"
)

func TestPromoteDrillDoesNotCallHostPython(t *testing.T) {
	b, err := os.ReadFile("../../scripts/ha/run-promote-drill.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if !strings.Contains(trim, "python") {
			continue
		}
		if strings.Contains(trim, "docker exec") || strings.Contains(trim, "docker run") {
			continue
		}
		t.Fatalf("host python call: %s", trim)
	}
}
