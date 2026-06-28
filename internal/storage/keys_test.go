package storage

import "testing"

func TestValidateObjectKey(t *testing.T) {
	valid := []string{"folder/file.txt", "a", "nested/path/to/object"}
	for _, k := range valid {
		if err := ValidateObjectKey(k); err != nil {
			t.Fatalf("%q: %v", k, err)
		}
	}
	invalid := []string{
		"",
		"../../etc/passwd",
		"/leading",
		`back\slash`,
		"bad\x00key",
	}
	for _, k := range invalid {
		if err := ValidateObjectKey(k); err == nil {
			t.Fatalf("%q should be invalid", k)
		}
	}
}

func TestValidateObjectKey_maxLength(t *testing.T) {
	key := stringsRepeat("a", 1025)
	if err := ValidateObjectKey(key); err == nil {
		t.Fatal("expected length error")
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
