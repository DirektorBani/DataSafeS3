package main

import "testing"

func TestRedact(t *testing.T) {
	out := redactMap(map[string]any{
		"username": "alice",
		"password": "hunter2",
		"token":    "jwt-value",
		"nested":   map[string]any{"exchange_code": "once"},
	})
	if out["username"] != "alice" || out["password"] != "[redacted]" || out["token"] != "[redacted]" {
		t.Fatalf("%#v", out)
	}
	nested, _ := out["nested"].(map[string]any)
	if nested["exchange_code"] != "[redacted]" {
		t.Fatalf("%#v", nested)
	}
	if !lineHasSecret("sees hunter2", "hunter2") {
		t.Fatal("detector missed the secret")
	}
}
