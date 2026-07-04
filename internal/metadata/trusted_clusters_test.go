package metadata

import "testing"

func TestHashPairingToken(t *testing.T) {
	raw := PairingTokenPrefix + "deadbeef"
	h := HashPairingToken(raw)
	if h == "" || h == raw {
		t.Fatal("expected hash")
	}
	if HashPairingToken(raw) != h {
		t.Fatal("hash not deterministic")
	}
}

func TestSafetyNumber(t *testing.T) {
	sn := SafetyNumber("aa", "bb")
	if len(sn) != 8 {
		t.Fatalf("safety number len %d", len(sn))
	}
	if SafetyNumber("bb", "aa") != sn {
		t.Fatal("order independent")
	}
}
