package auth

import (
	"os"
	"strings"
	"testing"
)

func TestDangerousActionsMatchDocs(t *testing.T) {
	for _, path := range []string{
		"../../docs/operations-guide/en/access-model.md",
		"../../docs/operations-guide/ru/access-model.md",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		const begin = "<!-- dangerous-actions:begin -->"
		const end = "<!-- dangerous-actions:end -->"
		text := string(body)
		i := strings.Index(text, begin)
		j := strings.Index(text, end)
		if i < 0 || j < 0 || j < i {
			t.Fatalf("%s missing dangerous-actions markers", path)
		}
		var got []string
		for _, line := range strings.Split(text[i+len(begin):j], "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") {
				continue
			}
			cols := strings.Split(line, "|")
			if len(cols) < 3 {
				continue
			}
			action := strings.TrimSpace(cols[1])
			if action == "" || action == "action" {
				continue
			}
			got = append(got, action)
		}
		if len(got) != len(DangerousActions) {
			t.Fatalf("%s has %d actions, want %d: %v", path, len(got), len(DangerousActions), got)
		}
		for i, action := range DangerousActions {
			if got[i] != action {
				t.Fatalf("%s action %d is %q, want %q", path, i, got[i], action)
			}
		}
	}
}
