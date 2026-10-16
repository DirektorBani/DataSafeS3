package s3

import "testing"

func TestHeartbeatVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "1.6.0"},
		{"dev", "1.6.0"},
		{"DEV", "1.6.0"},
		{"1.5.0", "1.6.0"},
		{"v1.5.0", "1.6.0"},
		{"1.4.1", "1.6.0"},
		{"1.6.0", "1.6.0"},
		{"v1.6.0", "v1.6.0"},
		{"1.6.1", "1.6.1"},
		{"1.7.0", "1.7.0"},
	}
	for _, tc := range cases {
		if got := HeartbeatVersion(tc.in); got != tc.want {
			t.Fatalf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}
