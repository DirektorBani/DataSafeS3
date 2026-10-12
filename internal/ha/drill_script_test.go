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

func TestPromoteDrillDockerClientIsNotRedownloaded(t *testing.T) {
	b, err := os.ReadFile("../../scripts/ha/run-promote-drill.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if strings.Contains(body, `[[ ! -x "$STATE/bin/docker" ]]`) {
		t.Fatal("execute-bit check re-downloads the ELF client on Git Bash")
	}
	if !strings.Contains(body, `[[ ! -s "$STATE/bin/docker" ]]`) {
		t.Fatal("docker client guard must use -s")
	}
	for _, want := range []string{"--retry 5", "--retry-delay 3", "-C -", "HTTPS_PROXY"} {
		if !strings.Contains(body, want) {
			t.Fatalf("download missing %s", want)
		}
	}
}
