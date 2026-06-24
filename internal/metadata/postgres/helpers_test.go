package postgres

import "testing"

func TestOptionalTextEmptyIsNil(t *testing.T) {
	if optionalText("") != nil {
		t.Fatal("empty string should map to nil for nullable FK columns")
	}
	if optionalText("team-1") != "team-1" {
		t.Fatal("non-empty should pass through")
	}
}
