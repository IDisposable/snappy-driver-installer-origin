package matcher

import "testing"

func TestOSDecorationsCount(t *testing.T) {
	if len(OSDecorations) != 336 {
		t.Fatalf("len(OSDecorations) = %d, want 336", len(OSDecorations))
	}
}

func TestOSDecorationsSpotCheck(t *testing.T) {
	cases := map[int]string{
		0:   "nt.5",
		5:   "ntarm64.5",
		132: "nt.10....16299", // the odd 4-dot variant
		137: "ntarm64.10....16299",
		324: "nt", // bare, undecorated row
		325: "ntx86",
		329: "ntarm64",
		330: "nt..", // double-dot row
		335: "ntarm64..",
	}
	for i, want := range cases {
		if OSDecorations[i] != want {
			t.Errorf("OSDecorations[%d] = %q, want %q", i, OSDecorations[i], want)
		}
	}
}

func TestOSDecorationsAllUnique(t *testing.T) {
	seen := make(map[string]bool, len(OSDecorations))
	for _, s := range OSDecorations {
		if seen[s] {
			t.Errorf("duplicate entry: %q", s)
		}
		seen[s] = true
	}
}
