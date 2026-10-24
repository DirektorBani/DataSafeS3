package main

import "testing"

func TestParseIDs(t *testing.T) {
	cases := []struct {
		in       string
		uid, gid int
		bad      bool
	}{
		{"65532:65532", 65532, 65532, false},
		{"1000", 1000, 1000, false},
		{"1000:2000", 1000, 2000, false},
		{"nobody", 0, 0, true},
		{"1000:grp", 0, 0, true},
		{"-1", 0, 0, true},
	}
	for _, c := range cases {
		uid, gid, err := parseIDs(c.in)
		if (err != nil) != c.bad {
			t.Fatalf("%q: err=%v, want bad=%v", c.in, err, c.bad)
		}
		if !c.bad && (uid != c.uid || gid != c.gid) {
			t.Fatalf("%q: got %d:%d, want %d:%d", c.in, uid, gid, c.uid, c.gid)
		}
	}
}
