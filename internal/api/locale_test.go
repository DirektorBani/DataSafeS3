package api

import "testing"

func TestAllowedUILocale(t *testing.T) {
	for _, locale := range []string{"ru", "en"} {
		if !allowedUILocale(locale) {
			t.Fatalf("allowedUILocale(%q) = false", locale)
		}
	}
	for _, locale := range []string{"", "de", "fr", "es", "zh", "RU", "en-US", "ru-RU"} {
		if allowedUILocale(locale) {
			t.Fatalf("allowedUILocale(%q) = true", locale)
		}
	}
}
