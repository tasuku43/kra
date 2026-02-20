package cli

import "testing"

func TestFindAttachDetachTrigger_ControlByte(t *testing.T) {
	i, n := findAttachDetachTrigger([]byte("abc\x1dxyz"))
	if i != 3 || n != 1 {
		t.Fatalf("idx=%d len=%d, want idx=3 len=1", i, n)
	}
}

func TestFindAttachDetachTrigger_CSIU(t *testing.T) {
	i, n := findAttachDetachTrigger([]byte("ab\x1b[93;5uzz"))
	if i != 2 || n != len("\x1b[93;5u") {
		t.Fatalf("idx=%d len=%d, want idx=2 len=%d", i, n, len("\x1b[93;5u"))
	}
}

func TestTrailingDetachPrefixLength(t *testing.T) {
	got := trailingDetachPrefixLength([]byte("ab\x1b[93;"))
	if got != len("\x1b[93;") {
		t.Fatalf("hold=%d, want=%d", got, len("\x1b[93;"))
	}
}

func TestTrailingDetachPrefixLength_ZeroForNoPrefix(t *testing.T) {
	got := trailingDetachPrefixLength([]byte("hello"))
	if got != 0 {
		t.Fatalf("hold=%d, want=0", got)
	}
}
