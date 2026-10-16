package auth

import (
	"os"
	"strings"
	"testing"
)

func TestAccessModelMatchesDocs(t *testing.T) {
	want := strings.TrimSpace(AccessModelMarkdown())
	for _, path := range []string{
		"../../docs/operations-guide/en/access-model.md",
		"../../docs/operations-guide/ru/access-model.md",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		const begin = "<!-- access-matrix:begin -->"
		const end = "<!-- access-matrix:end -->"
		text := string(body)
		i := strings.Index(text, begin)
		j := strings.Index(text, end)
		if i < 0 || j < 0 || j < i {
			t.Fatalf("%s missing access matrix markers", path)
		}
		got := strings.TrimSpace(text[i+len(begin) : j])
		if got != want {
			t.Fatalf("%s matrix diverges from internal/auth/access_matrix.go", path)
		}
	}
}
