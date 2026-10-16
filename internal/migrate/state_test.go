package migrate

import "testing"

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Has("b", "k", "v1") {
		t.Fatal("empty state reported a hit")
	}
	if err := st.Mark("b", "k", "v1", "copied", "abc", 3); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenState(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !again.Has("b", "k", "v1") {
		t.Fatal("copied row was not reloaded")
	}
	if again.Has("b", "k", "v2") {
		t.Fatal("unexpected version")
	}
}
