package promote

import "testing"

func TestExitCodesAreDistinct(t *testing.T) {
	codes := []int{ExitOK, ExitUsage, ExitPrecheck, ExitFence, ExitPromote, ExitPostcheck}
	seen := map[int]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate exit code %d", c)
		}
		seen[c] = true
	}
	if ExitOK != 0 || ExitUsage != 1 || ExitPrecheck != 2 || ExitFence != 3 || ExitPromote != 4 || ExitPostcheck != 5 {
		t.Fatalf("exit codes changed: %v", codes)
	}
}
